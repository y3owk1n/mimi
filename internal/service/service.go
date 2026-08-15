package service

import (
	"context"
	"os"
	"os/user"
	"path/filepath"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/paths"
)

const (
	dirPerm  = 0o755
	filePerm = 0o644
)

// foreignInstallAdvice is the tail of every refusal to touch a service mimi
// did not install. It names the tools that install one of their own, because
// the fix is always in their configuration and never in mimi.
const foreignInstallAdvice = "check for existing installations (e.g., nix-darwin, home-manager) and uninstall them first"

// Status is what checking the launchd service reports: a typed result a
// caller can act on, in place of a formatted string only a human can read.
type Status struct {
	// Loaded reports whether launchctl currently has the service loaded.
	Loaded bool
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
func (s *Service) Install(configPath, logFile string) (InstallOutcome, error) {
	ctx := context.Background()

	loaded := s.launcher.list(ctx, Label) == nil
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

	plistContent := renderPlist(binPath, configPath, logFile)

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

// Uninstall unloads the service and removes its plist. A launchctl bootout
// failure is not fatal — the service may already be unloaded, e.g. after a
// prior partial uninstall — so it only removes the plist file.
func (s *Service) Uninstall() error {
	ctx := context.Background()

	domain, err := guiDomain()
	if err != nil {
		return err
	}

	_ = s.launcher.bootout(ctx, domain+"/"+Label)

	err = os.Remove(plistPath())
	if err != nil && !os.IsNotExist(err) {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "removing plist")
	}

	return nil
}

// Start starts the already-installed service.
func (s *Service) Start() error {
	err := s.launcher.start(context.Background(), Label)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "starting service")
	}

	return nil
}

// Stop stops the running service.
func (s *Service) Stop() error {
	err := s.launcher.stop(context.Background(), Label)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "stopping service")
	}

	return nil
}

// Restart stops then starts the service. A failure to stop (e.g. it was not
// running) does not prevent the start that follows.
func (s *Service) Restart() error {
	_ = s.Stop()

	return s.Start()
}

// Status reports whether the service is currently loaded.
func (s *Service) Status() Status {
	return Status{Loaded: s.launcher.list(context.Background(), Label) == nil}
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

		err = s.launcher.bootout(ctx, domain+"/"+Label)
		if err != nil {
			return 0, derrors.Wrapf(
				err,
				derrors.CodeServiceFailed,
				"unloading the previous service",
			)
		}
	}

	err = writePlist(path, content)
	if err != nil {
		return 0, err
	}

	err = s.launcher.bootstrap(ctx, domain, path)
	if err != nil {
		return 0, derrors.Wrapf(err, derrors.CodeServiceFailed, "loading service")
	}

	return outcome, nil
}

// installedPlist is what sits at mimi's plist path when an install starts.
type installedPlist struct {
	// path is where this was read from.
	path string
	// present reports whether there is anything at path at all.
	present bool
	// regular reports whether what is there is a plain file. It is a weaker
	// claim than "mimi wrote it" and deliberately so: a symlink is
	// home-manager's link into the Nix store, and that is the shape mimi can
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

// binaryPath resolves the real, symlink-free path to the running mimi
// binary, which is what the installed plist must invoke.
func binaryPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	return filepath.EvalSymlinks(execPath)
}
