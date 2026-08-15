package service

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"time"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/paths"
)

const (
	dirPerm  = 0o755
	filePerm = 0o644
)

// How long an install waits for the service it booted out to actually be gone,
// and how often it looks.
//
// launchctl bootout returns once launchd has accepted the request, not once
// the job has finished exiting, so the wait is for the daemon's own shutdown:
// mimi's closes an observer, a socket and a log, and a second is already
// generous for that. Five is wide enough to cover a machine under load without
// being long enough that a wedged daemon reads as a hung terminal. The
// interval is short because the common case ends on the first poll, and every
// poll is one cheap `launchctl list`.
const (
	unloadTimeout      = 5 * time.Second
	unloadPollInterval = 50 * time.Millisecond
	// unloadPollAttempts is the timeout counted in polls, since polls are what
	// the wait actually spends. The first is taken before any pause, so the
	// last one lands an interval short of the timeout rather than past it.
	unloadPollAttempts = int(unloadTimeout / unloadPollInterval)
)

// foreignInstallAdvice is the tail of every refusal to touch a service mimi
// did not install. It claims exactly the reach the checks in front of it have,
// which is one launchd label: mimi's own.
//
// It used to send the user looking for a nix-darwin or home-manager install,
// and those two are the installs it can never be reporting. Both checks only
// ever ask about [Label] — the plist at its path, and the job launchd holds
// under it — while the modules in this repo register their agent under labels
// of their own: nix/darwin.nix through launchd.user.agents.mimi, nix/home.nix
// through launchd.agents.mimi. Naming them promised a conflict check that was
// never performed, so what fires is a stale mimi install or a hand-written
// plist. Saying what was looked at leaves the rest of the search where it can
// actually be described, in docs/INSTALLATION.md.
const foreignInstallAdvice = "remove it with whatever installed it, then run install again — " +
	"only mimi's own label (" + Label + ") was checked, so an installation under another label is not seen"

// LoadState is what asking launchd whether the service is loaded produced.
//
// It exists because that question has three answers and not two. `launchctl
// list` exits non-zero both for a job launchd does not hold and for a
// launchctl that could not run at all, and reading the second as the first is
// how a service that is still up gets treated as gone.
type LoadState int

// These count from zero, where [InstallOutcome] counts from one, and the zero
// value is why: a load state nobody set must not read as the service being
// gone. Every caller here does something safe with "I do not know" and
// something destructive with a wrong "no".
const (
	// LoadStateUnknown is launchctl never having run: not on PATH, or unable
	// to be spawned.
	LoadStateUnknown LoadState = iota
	// LoadStateLoaded is a job launchd currently holds.
	LoadStateLoaded
	// LoadStateNotLoaded is launchctl running and reporting no such job.
	LoadStateNotLoaded
)

// Status is what checking the launchd service reports: a typed result a
// caller can act on, in place of a formatted string only a human can read.
//
// State is the only field always answered, and even it may be
// [LoadStateUnknown]. The installed plist sets KeepAlive with a ten second
// ThrottleInterval, so a daemon that crashes at startup is relaunched forever
// and stays as loaded as a healthy one — the other two fields are what tell
// them apart, and both are optional because launchd's own description of the
// job is undocumented text that may not carry them.
type Status struct {
	// State is what launchctl said when asked whether the service is loaded,
	// including its having been unable to say.
	State LoadState
	// PID is the process id of the running daemon. It is unknown whenever the
	// service is not loaded, is not currently running, or launchd's
	// description could not be read.
	PID OptionalInt
	// LastExitStatus is how the daemon last exited. It is unknown before it
	// ever has, when it was killed by a signal rather than exiting, and
	// whenever launchd's description could not be read.
	LastExitStatus OptionalInt
	// CapturedStdout and CapturedStderr are the console streams the installed
	// plist captures, as that plist names them. Both are zero when there is no
	// plist of mimi's to read them from — nothing installed, or a plist another
	// installer wrote.
	CapturedStdout CapturedLog
	CapturedStderr CapturedLog
}

// CapturedLog is one of the daemon's captured console streams: where the
// installed plist tells launchd to write it, and how much has been written.
//
// The size is the fact worth having. Nothing rotates these two files — launchd
// holds them open and appends — so the daemon empties them once per start, and
// a size is therefore one run's console output. A large one under a service
// that keeps exiting is the crash loop this status exists to expose.
type CapturedLog struct {
	// Path is where launchd was told to write this stream. Empty when it could
	// not be read from the installed plist.
	Path string
	// Size is how many bytes the file holds. It means nothing unless Present.
	Size int64
	// Present reports whether the file is there at all. launchd creates it when
	// it first spawns the daemon, so an absent one is a service that has never
	// run — which is not the same answer as a stream that ran and said nothing.
	Present bool
}

// InstallOutcome is what an [Service.Install] did. Install is idempotent, so
// "it succeeded" is no longer enough for a caller to tell the user what
// happened: the same command creates a service, brings a stale one back in
// line with the config, or does nothing at all.
type InstallOutcome int

// The outcomes count from one so that the zero value is none of them: an
// outcome nobody set is not silently the one that sorts first.
const (
	// InstallOutcomeInstalled is a service that was not loaded and now is.
	InstallOutcomeInstalled InstallOutcome = iota + 1
	// InstallOutcomeReplaced is a loaded service whose plist disagreed with
	// the config describing it: the plist was replaced and the service
	// reloaded so launchd reads the new one.
	InstallOutcomeReplaced
	// InstallOutcomeUnchanged is a loaded service whose plist already is the
	// bytes mimi would render. Nothing was written and nothing was reloaded.
	InstallOutcomeUnchanged
)

// Service manages the mimi launchd service. Every launchctl invocation runs
// through the launcher it holds.
type Service struct {
	launcher launcher
	// sleep is how the pause between unload polls is spent. Only tests set
	// it; a nil sleep is [time.Sleep], so the poll loop can be driven without
	// spending the wall-clock time its bound is written in.
	sleep func(time.Duration)
}

// New returns a Service backed by the real launchctl binary on PATH.
func New() *Service {
	return &Service{launcher: execLauncher{}}
}

// Install renders the plist for the running binary and configPath, writes it
// to ~/Library/LaunchAgents, and loads it with launchctl bootstrap.
//
// It is idempotent, and that is the point: the plist is a snapshot of the
// config taken at install time, so running install again is how an installed
// service is brought back in line with a config that has moved since. A plist
// that already matches is left alone and the loaded service is not disturbed.
//
// logFile is settings.log_file as config resolved it, and is only used to
// place the daemon's captured stdout/stderr alongside it; an empty value is
// valid and leaves those streams at their /tmp defaults.
//
// servicePath is settings.service_path: the PATH the installed service, and so
// every hook it runs, is given. An empty value is valid and leaves the PATH
// the plist has always carried. It is baked in here and nowhere else, which is
// what makes it reinstall-only
// (docs/adr/0003-a-setting-the-daemon-never-reads-is-reinstall-only.md).
//
// It refuses outright when launchctl cannot say whether the service is loaded.
// That answer is what both of its safety checks rest on — whether this label
// already belongs to another installer, and whether the running job has to be
// unloaded before launchd will read a new plist — and an install that assumed
// "not loaded" would bootstrap over a service that may well be up. Refusing
// costs nothing that a re-run does not recover: it happens before anything is
// written, and install is idempotent.
//
// Canceling ctx ends the install, and where it ends is the same place any
// other failure does: either before the new plist is written, leaving the old
// one for the next install to disagree with, or after the plist is in place
// with only the load left to retry. There is no third state.
func (s *Service) Install(
	ctx context.Context,
	configPath, logFile, servicePath string,
) (InstallOutcome, error) {
	state, err := s.loadState(ctx)
	if err != nil {
		return 0, err
	}

	loaded := state == LoadStateLoaded
	expandedPlist := plistPath()

	installed, err := readInstalledPlist(expandedPlist)
	if err != nil {
		return 0, err
	}

	err = installed.refuseForeign(loaded)
	if err != nil {
		return 0, err
	}

	binPath, err := binaryPath()
	if err != nil {
		return 0, derrors.Wrapf(err, derrors.CodeServiceFailed, "getting binary path")
	}

	plistContent := renderPlist(binPath, configPath, logFile, servicePath)

	// launchd opens StandardOutPath and StandardErrorPath at spawn time and
	// creates no directories of its own: a missing one silently discards the
	// console output of the very first run, which is the startup crash these
	// streams exist to capture. mimi's own file log only creates the
	// directory later, once the logger is built. This runs before the
	// unchanged check on purpose — a directory deleted since the last install
	// is exactly the drift a re-run should repair.
	captureDir := capturedStreamsFor(logFile).dir()

	err = os.MkdirAll(captureDir, dirPerm)
	if err != nil {
		return 0, derrors.Wrapf(err, derrors.CodeServiceFailed, "creating log directory")
	}

	if loaded && installed.matches(plistContent) {
		return InstallOutcomeUnchanged, nil
	}

	err = os.MkdirAll(filepath.Dir(expandedPlist), dirPerm)
	if err != nil {
		return 0, derrors.Wrapf(err, derrors.CodeServiceFailed, "creating LaunchAgents directory")
	}

	return s.apply(ctx, loaded, expandedPlist, plistContent)
}

// Uninstall unloads the service and removes its plist.
//
// The bootout is attempted either way, but whether its failure matters is
// decided before it runs. An unloaded service — after a prior partial
// uninstall, say — has nothing to unload, and a bootout that fails at nothing
// must not stand between the leftover plist and its removal. A loaded service
// whose bootout failed is a different thing wearing the same error: it is
// still running, and it keeps running until logout. Removing its plist there
// would take away the only thing an uninstall can still act on, so the file
// stays and the failure is returned for the caller to retry.
//
// Tolerating that failure takes a positive answer that there was nothing to
// unload, not merely the absence of one. A launchctl that could not run leaves
// the same question open as a loaded service does, and the safe reading of an
// open question here is the one that keeps the plist. So does a canceled ctx:
// it stops the uninstall where a failed unload does, with the plist still on
// disk and a retry all it takes.
func (s *Service) Uninstall(ctx context.Context) error {
	target, err := serviceTarget()
	if err != nil {
		return err
	}

	// Sampled before the bootout, so a service that unloads between the two
	// makes this uninstall report a failure the retry will not see again.
	// Sampling after would be worse: the bootout's own success is what empties
	// it, so every unload would look like it had nothing to unload.
	//
	// Why it could not be sampled is deliberately not returned: the bootout
	// that follows may well succeed, and an uninstall that worked has nothing
	// to report. It changes what a failure says, though — "unloading failed,
	// try again" is the wrong advice when what is broken is the launchctl
	// both halves of this run through.
	state, _ := s.loadState(ctx)

	err = s.launcher.bootout(ctx, target)

	// A canceled uninstall stops here, whatever the bootout reported. Removing
	// the plist below would finish the very command the caller called off, and
	// a bootout that a cancellation cut short says nothing about the service
	// either — both readings below would be about a launchctl that was never
	// allowed to answer.
	ctxErr := ctx.Err()
	if ctxErr != nil {
		return derrors.Wrapf(
			ctxErr,
			derrors.CodeServiceFailed,
			"uninstall canceled; the plist is kept, so running uninstall again finishes it",
		)
	}

	if err != nil && state == LoadStateUnknown {
		return derrors.Wrapf(
			err,
			derrors.CodeServiceFailed,
			"unloading service; launchctl could not say whether it was loaded, "+
				"so the plist is kept and this failure stands",
		)
	}

	if err != nil && state == LoadStateLoaded {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "unloading service")
	}

	err = os.Remove(plistPath())
	if err != nil && !os.IsNotExist(err) {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "removing plist")
	}

	return nil
}

// Start starts the already-installed service.
func (s *Service) Start(ctx context.Context) error {
	err := s.launcher.start(ctx, Label)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "starting service")
	}

	return nil
}

// Stop stops the running service.
func (s *Service) Stop(ctx context.Context) error {
	err := s.launcher.stop(ctx, Label)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "stopping service")
	}

	return nil
}

// Restart restarts the loaded service in a single launchd call.
//
// It used to be a stop whose error was discarded followed by a start, and the
// discard was not the worst of it: on a machine with nothing installed both
// halves fail at nothing, and the one failure that survived reported "starting
// service" — the symptom of the absent service rather than the absent service.
// A kickstart is the whole restart, so there is no first error left to throw
// away and no way to name the second one's problem instead of the real one.
//
// That is also why the loaded check comes first and refuses on anything but a
// loaded job. launchd will not kickstart a job it does not hold, so the check
// costs a command that would have failed anyway and buys the one thing the
// failure could never say: that the fix is `mimi services install` and not
// anything about restarting. [LoadStateUnknown] is not that answer — a
// launchctl that could not run has said nothing about whether a service is
// installed, and it is returned as itself rather than turned into advice to
// install over a service that may be running perfectly well.
//
// Start and Stop are left as they are. Each is a command of its own, and a
// launchctl start on an unloaded job is a different thing from a restart of
// one.
func (s *Service) Restart(ctx context.Context) error {
	state, err := s.loadState(ctx)
	if err != nil {
		return err
	}

	if state == LoadStateNotLoaded {
		return derrors.New(
			derrors.CodeServiceFailed,
			"there is no loaded service to restart; run `mimi services install` first",
		)
	}

	target, err := serviceTarget()
	if err != nil {
		return err
	}

	err = s.launcher.kickstart(ctx, target)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "restarting service")
	}

	return nil
}

// Status reports whether the service is currently loaded, and — when it is —
// the pid it runs under or the status it last exited with, plus the captured
// console streams the installed plist names and how large they have grown.
//
// Nothing here is guaranteed, and the load state least of all: it is the one
// thing this always reports, and [LoadStateUnknown] is one of the things it can
// report. That is the same bargain the rest of the status makes — everything
// past the load state comes from `launchctl print`, whose output Apple
// documents nowhere, so anything that goes wrong there degrades to the answer
// this returned before it asked. A status command that fails tells a user less
// than one that says a little less, and that holds for a launchctl that could
// not run too: unknown is a thing to report, not a reason to report nothing.
//
// The captured streams are read whether or not the service is loaded: they are
// files on disk, and the run that wrote them is over either way — an unloaded
// service is one of the states in which their contents matter most.
func (s *Service) Status(ctx context.Context) Status {
	// The reason launchctl could not answer is not carried out of here: this
	// returns no error by design, and [LoadStateUnknown] is already the whole
	// of what a caller can do about it.
	state, _ := s.loadState(ctx)

	status := Status{State: state}
	status.CapturedStdout, status.CapturedStderr = installedCapturedLogs()

	if status.State != LoadStateLoaded {
		return status
	}

	target, err := serviceTarget()
	if err != nil {
		return status
	}

	output, err := s.launcher.printJob(ctx, target)
	if err != nil {
		return status
	}

	report := parseJobReport(output)
	status.PID = report.pid
	status.LastExitStatus = report.lastExitStatus

	return status
}

// loadState asks launchd whether the service is loaded, and is the only place
// in this package that asks.
//
// Everything that reads it — install, uninstall, status, and the wait between
// an unload and the load after it — used to take a failed `launchctl list` for
// a job that is not there. They want different things from the third state, so
// what they share is this: one call that names it, and no caller left able to
// miss it.
//
// The error is returned alongside because it is the whole of what
// [LoadStateUnknown] means, and it is non-nil exactly then. A caller that
// refuses to act without an answer returns it and says why launchctl could not
// give one; a caller that only needs to know whether it has one reads the
// state and lets the error go.
func (s *Service) loadState(ctx context.Context) (LoadState, error) {
	loaded, err := s.launcher.list(ctx, Label)
	if err != nil {
		return LoadStateUnknown, derrors.Wrapf(
			err,
			derrors.CodeServiceFailed,
			"asking launchctl whether the service is loaded",
		)
	}

	if loaded {
		return LoadStateLoaded, nil
	}

	return LoadStateNotLoaded, nil
}

// apply makes the installed service be content: it unloads whatever is
// loaded, replaces the plist at path, and hands the new one to launchd.
//
// The order is the whole of it. launchd holds the plist it was bootstrapped
// with and re-reads one only when the job is loaded again, so replacing the
// file alone changes nothing — but writing before unloading is worse than
// useless. A bootout that failed after the write would leave the new plist on
// disk in front of a service still running the old one, and every later
// install would find the file it wanted and report the service up to date,
// forever. Unloading first means a failure leaves the old plist untouched, so
// the next install sees the same disagreement this one did and retries it.
//
// The wait between the two is the same argument in time rather than order:
// see [Service.waitForUnload].
func (s *Service) apply(
	ctx context.Context,
	loaded bool,
	path, content string,
) (InstallOutcome, error) {
	domain, err := guiDomain()
	if err != nil {
		return 0, err
	}

	outcome := InstallOutcomeInstalled

	if loaded {
		outcome = InstallOutcomeReplaced

		err = s.launcher.bootout(ctx, targetIn(domain))
		if err != nil {
			return 0, derrors.Wrapf(
				err,
				derrors.CodeServiceFailed,
				"unloading the previous service",
			)
		}

		err = s.waitForUnload(ctx)
		if err != nil {
			return 0, err
		}
	}

	// The write is the point of no return: everything above it can be undone by
	// running install again, and nothing below it leaves the old plist behind.
	// An interrupt that arrived while the unload was being waited on has to
	// stop on this side of it, or a command the user called off is the one that
	// replaced their plist.
	err = ctx.Err()
	if err != nil {
		return 0, derrors.Wrapf(
			err,
			derrors.CodeServiceFailed,
			"install canceled before the new plist was written; the previous plist is untouched",
		)
	}

	err = writePlist(path, content)
	if err != nil {
		return 0, err
	}

	err = s.launcher.bootstrap(ctx, domain, path)
	if err != nil {
		// The plist is already the new one by now, so this failed with the
		// config change made and only the load left undone — the one failure
		// here that a plain re-run fixes, and the user is the only one who
		// can order it.
		return 0, derrors.Wrapf(
			err,
			derrors.CodeServiceFailed,
			"loading service; the new plist is already in place, so running install again retries the load",
		)
	}

	return outcome, nil
}

// waitForUnload blocks until launchctl stops listing the service, or until
// the poll bound above runs out.
//
// A bootout is a request, not a result: launchd takes it, sends the daemon on
// its way and answers immediately, so the job can still be there for as long
// as it takes that process to exit. Bootstrapping in that window fails with
// "Operation now in progress" — a real failure, over a service that was only
// slow. Waiting here is what turns the common case of a daemon taking a
// moment back into an install that simply works.
//
// Only launchctl saying the job is gone ends the wait. A launchctl that cannot
// run says nothing about the job, and taking that for the job being gone would
// hand the bootstrap straight back into the window this exists to close — so
// it gives up instead, and says why. Waiting out the rest of the bound would
// only spend the timeout to reach the same place with a vaguer reason: the
// answer failed to arrive, and it is not the daemon that is slow.
//
// A canceled ctx ends it too, at the next pause rather than at the bound: this
// is the one place an install blocks, so a caller who has given up is a caller
// waiting on this and nothing else.
func (s *Service) waitForUnload(ctx context.Context) error {
	for attempt := range unloadPollAttempts {
		if attempt > 0 {
			err := s.pause(ctx, unloadPollInterval)
			if err != nil {
				return derrors.Wrapf(
					err,
					derrors.CodeServiceFailed,
					"canceled while waiting for the previous service to unload; "+
						"the previous plist is untouched",
				)
			}
		}

		state, err := s.loadState(ctx)
		if err != nil {
			return derrors.Wrapf(
				err,
				derrors.CodeServiceFailed,
				"waiting for the previous service to unload",
			)
		}

		if state == LoadStateNotLoaded {
			return nil
		}
	}

	return derrors.Newf(
		derrors.CodeServiceFailed,
		"the previous service was still loaded %s after it was unloaded; "+
			"stop whatever is holding it and run install again",
		unloadTimeout,
	)
}

// pause spends the interval between two unload polls, and returns why it
// stopped early when the caller canceled during it. A Service with no sleep of
// its own — every one outside this package's tests — spends it for real, and
// gives up on the interval the moment ctx is done rather than at the end of it.
//
// The injected sleep is the tests' and costs nothing, so the context is asked
// after it rather than raced against it: a test that cancels from inside its
// sleep is describing an interrupt that arrived during the pause, and gets the
// same answer a real one would.
func (s *Service) pause(ctx context.Context, interval time.Duration) error {
	if s.sleep != nil {
		s.sleep(interval)

		return ctx.Err()
	}

	timer := time.NewTimer(interval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// installedPlist is what sits at mimi's plist path when an install starts.
type installedPlist struct {
	// path is where this was read from.
	path string
	// present reports whether there is anything at path at all.
	present bool
	// regular reports whether what is there is a plain file. It is a weaker
	// claim than "mimi wrote it" and deliberately so: a symlink is the shape a
	// plist linked out of the Nix store has, and that is the shape mimi can
	// actually tell apart. Anything else at mimi's own path is treated as
	// mimi's to replace.
	regular bool
	// content is the file's current bytes, empty unless regular.
	content string
}

// readInstalledPlist describes what is at path without changing it. A path
// with nothing at it is not an error: that is the first install.
func readInstalledPlist(path string) (installedPlist, error) {
	// Lstat, not Stat: a symlink must be seen as a symlink rather than as
	// whatever it points at.
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return installedPlist{path: path}, nil
		}

		return installedPlist{}, derrors.Wrapf(err, derrors.CodeServiceFailed, "inspecting plist")
	}

	if !info.Mode().IsRegular() {
		return installedPlist{path: path, present: true}, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return installedPlist{}, derrors.Wrapf(err, derrors.CodeServiceFailed, "reading plist")
	}

	return installedPlist{path: path, present: true, regular: true, content: string(content)}, nil
}

// refuseForeign reports the two shapes that mean this label belongs to an
// installer other than mimi: something at mimi's plist path that mimi could
// not have written, and a loaded service with no plist of mimi's behind it.
// Overwriting either would fight whatever manages it, and lose on the next
// rebuild.
func (p installedPlist) refuseForeign(loaded bool) error {
	if p.present && !p.regular {
		return derrors.Newf(
			derrors.CodeServiceFailed,
			"%s is not a file mimi wrote; %s",
			p.path,
			foreignInstallAdvice,
		)
	}

	if loaded && !p.present {
		return derrors.Newf(
			derrors.CodeServiceFailed,
			"service is already loaded but mimi did not install it; %s",
			foreignInstallAdvice,
		)
	}

	return nil
}

// matches reports whether the plist on disk already is the bytes rendered for
// this install, which is what makes a re-run a no-op.
func (p installedPlist) matches(content string) bool {
	return p.present && p.content == content
}

// writePlist puts content at path by writing a temporary file in the same
// directory and renaming it over the target. Rename within a directory
// replaces the name in one step, so an install interrupted at any point
// leaves either the old plist or the new one — never a truncated file for
// launchd to reject on the next login.
func writePlist(path, content string) error {
	dir := filepath.Dir(path)

	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "creating plist")
	}

	tmpPath := tmpFile.Name()

	_, err = tmpFile.WriteString(content)
	if err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)

		return derrors.Wrapf(err, derrors.CodeServiceFailed, "writing plist")
	}

	err = tmpFile.Close()
	if err != nil {
		_ = os.Remove(tmpPath)

		return derrors.Wrapf(err, derrors.CodeServiceFailed, "closing plist")
	}

	// CreateTemp makes the file 0600, which is not the mode a LaunchAgents
	// plist has ever had here.
	err = os.Chmod(tmpPath, filePerm)
	if err != nil {
		_ = os.Remove(tmpPath)

		return derrors.Wrapf(err, derrors.CodeServiceFailed, "setting plist permissions")
	}

	err = os.Rename(tmpPath, path)
	if err != nil {
		_ = os.Remove(tmpPath)

		return derrors.Wrapf(err, derrors.CodeServiceFailed, "replacing plist")
	}

	return nil
}

// plistPath is where mimi's own plist lives, expanded. Install and Uninstall
// have to name the same file, so neither builds the path itself.
func plistPath() string {
	return filepath.Join(paths.ExpandHome(launchAgentsDir), Label+".plist")
}

// guiDomain is the launchd domain a per-user agent belongs to, for the user
// running mimi. Every launchctl target mimi names is built from it.
func guiDomain() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", derrors.Wrapf(err, derrors.CodeServiceFailed, "getting current user")
	}

	return "gui/" + currentUser.Uid, nil
}

// targetIn is mimi's job as launchctl names it, under a domain already in
// hand. Every call that takes a domain target — bootout, print, kickstart —
// names the same job, so none of them spells it out for itself.
//
// It is not what launchctl start and stop take: those two predate domain
// targets and take the bare label.
func targetIn(domain string) string {
	return domain + "/" + Label
}

// serviceTarget is [targetIn] for the caller that has no domain yet, which is
// every one of them but [Service.apply] — that one holds the domain already,
// because bootstrap is the one call that takes it bare.
func serviceTarget() (string, error) {
	domain, err := guiDomain()
	if err != nil {
		return "", err
	}

	return targetIn(domain), nil
}

// binaryPath resolves the real, symlink-free path to the running mimi
// binary, which is what the installed plist must invoke.
func binaryPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	return filepath.EvalSymlinks(execPath)
}
