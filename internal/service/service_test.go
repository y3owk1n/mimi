//nolint:testpackage // launcher and Service's internals are unexported; the seam under test is intentionally internal.
package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// testConfigPath is the config path the install tests hand the plist
// renderer. Only its presence in the rendered bytes matters, so every test
// that does not vary it uses this one.
const testConfigPath = "/Users/test/.config/mimi/config.toml"

// fakeLauncher is a launcher made of plain values: every call it sees lands
// in a field a test can assert against, and every call it returns is a field
// a test can set up in advance.
type fakeLauncher struct {
	loaded bool

	// calls names the state-changing launchctl calls in the order they
	// arrived, for the tests where "which, and in what order" is the
	// behavior under test. The read-only list is deliberately absent: it
	// changes nothing, so a caller making it is not an observable effect.
	calls []string

	bootstrapErr error
	bootstrapped bool
	bootoutErr   error
	booted       bool
	startErr     error
	started      bool
	stopErr      error
	stopped      bool

	// printOutput and printErr stand in for `launchctl print`, whose stdout —
	// unlike every other call here — is the point of making it.
	printOutput string
	printErr    error
	printedFor  string
}

func (f *fakeLauncher) list(_ context.Context, _ string) error {
	if f.loaded {
		return nil
	}

	return derrors.New(derrors.CodeServiceFailed, "not loaded")
}

func (f *fakeLauncher) printJob(_ context.Context, target string) (string, error) {
	f.printedFor = target

	return f.printOutput, f.printErr
}

func (f *fakeLauncher) bootstrap(_ context.Context, _, _ string) error {
	f.bootstrapped = true
	f.calls = append(f.calls, "bootstrap")

	return f.bootstrapErr
}

func (f *fakeLauncher) bootout(_ context.Context, _ string) error {
	f.booted = true
	f.calls = append(f.calls, "bootout")

	return f.bootoutErr
}

func (f *fakeLauncher) start(_ context.Context, _ string) error {
	f.started = true
	f.calls = append(f.calls, "start")

	return f.startErr
}

func (f *fakeLauncher) stop(_ context.Context, _ string) error {
	f.stopped = true
	f.calls = append(f.calls, "stop")

	return f.stopErr
}

// TestService_Status_ReportsWhatTheLauncherSees covers the distinction the
// status exists to make: a loaded service is not necessarily a running one.
// The installed plist sets KeepAlive with a ten second ThrottleInterval, so a
// daemon that crashes at startup stays loaded forever while never running, and
// only the pid and the last exit status tell it apart from a healthy one.
func TestService_Status_ReportsWhatTheLauncherSees(t *testing.T) {
	tests := []struct {
		name        string
		loaded      bool
		printOutput string
		printErr    error
		want        Status
	}{
		{
			name:        "loaded and running",
			loaded:      true,
			printOutput: "\tstate = running\n\tpid = 1478\n\tlast exit code = (never exited)\n",
			want:        Status{Loaded: true, PID: OptionalInt{Value: 1478, Known: true}},
		},
		{
			name:        "loaded but respawning after a crash",
			loaded:      true,
			printOutput: "\tstate = spawn scheduled\n\tlast exit code = 1\n",
			want:        Status{Loaded: true, LastExitStatus: OptionalInt{Value: 1, Known: true}},
		},
		{
			// launchctl print is undocumented text with no promise behind it,
			// so a shape this parser does not know must cost the command
			// nothing beyond the detail it could not read.
			name:        "loaded, with output nothing can be read from",
			loaded:      true,
			printOutput: "some future launchctl saying something else entirely",
			want:        Status{Loaded: true},
		},
		{
			name:     "loaded, with launchctl print failing outright",
			loaded:   true,
			printErr: derrors.New(derrors.CodeServiceFailed, "no such process"),
			want:     Status{Loaded: true},
		},
		{
			// Nothing to describe, so nothing is asked.
			name:        "not loaded",
			loaded:      false,
			printOutput: "\tpid = 1478\n",
			want:        Status{Loaded: false},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fake := &fakeLauncher{
				loaded:      testCase.loaded,
				printOutput: testCase.printOutput,
				printErr:    testCase.printErr,
			}
			svc := &Service{launcher: fake}

			got := svc.Status()
			if got != testCase.want {
				t.Errorf("Status() = %+v, want %+v", got, testCase.want)
			}

			if !testCase.loaded && fake.printedFor != "" {
				t.Errorf("Status() described an unloaded service: %q", fake.printedFor)
			}

			// launchctl print takes a domain target, not the bare label a
			// launchctl list takes.
			if testCase.loaded && !strings.HasSuffix(fake.printedFor, "/"+Label) {
				t.Errorf(
					"Status() printed %q, want a domain target ending in /%s",
					fake.printedFor,
					Label,
				)
			}
		})
	}
}

func TestService_Start_RunsLaunchctlStart(t *testing.T) {
	fake := &fakeLauncher{}
	svc := &Service{launcher: fake}

	err := svc.Start()
	if err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}

	if !fake.started {
		t.Error("Start() did not call the launcher's start")
	}
}

func TestService_Start_WrapsTheLauncherErrorWithADerrorsCode(t *testing.T) {
	fake := &fakeLauncher{startErr: derrors.New(derrors.CodeServiceFailed, "boom")}
	svc := &Service{launcher: fake}

	err := svc.Start()
	if err == nil {
		t.Fatal("Start() = nil, want an error")
	}

	if derrors.GetCode(err) != derrors.CodeServiceFailed {
		t.Errorf("Start() code = %v, want %v", derrors.GetCode(err), derrors.CodeServiceFailed)
	}
}

func TestService_Stop_RunsLaunchctlStop(t *testing.T) {
	fake := &fakeLauncher{}
	svc := &Service{launcher: fake}

	err := svc.Stop()
	if err != nil {
		t.Fatalf("Stop() = %v, want nil", err)
	}

	if !fake.stopped {
		t.Error("Stop() did not call the launcher's stop")
	}
}

func TestService_Restart_StopsThenStartsEvenWhenStopFails(t *testing.T) {
	fake := &fakeLauncher{stopErr: derrors.New(derrors.CodeServiceFailed, "wasn't running")}
	svc := &Service{launcher: fake}

	err := svc.Restart()
	if err != nil {
		t.Fatalf("Restart() = %v, want nil (start succeeded)", err)
	}

	if !fake.stopped || !fake.started {
		t.Errorf("Restart() stopped=%v started=%v, want both true", fake.stopped, fake.started)
	}
}

func TestService_Uninstall_BootsOutEvenWhenNotLoaded(t *testing.T) {
	fake := &fakeLauncher{bootoutErr: derrors.New(derrors.CodeServiceFailed, "not loaded")}
	svc := &Service{launcher: fake}

	dir := t.TempDir()
	t.Setenv("HOME", dir)

	err := svc.Uninstall()
	if err != nil {
		t.Fatalf("Uninstall() = %v, want nil (a bootout failure is not fatal)", err)
	}

	if !fake.booted {
		t.Error("Uninstall() did not call the launcher's bootout")
	}
}

func TestService_Install_WritesThePlistAndBootstraps(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// A log directory that does not exist yet, as on a machine where the
	// service is installed before mimi has ever run.
	logDir := filepath.Join(dir, "state", "mimi")
	logFile := filepath.Join(logDir, "mimi.log")

	fake := &fakeLauncher{}
	svc := &Service{launcher: fake}

	outcome, err := svc.Install(testConfigPath, logFile)
	if err != nil {
		t.Fatalf("Install() = %v, want nil", err)
	}

	if outcome != InstallOutcomeInstalled {
		t.Errorf("Install() outcome = %v, want %v", outcome, InstallOutcomeInstalled)
	}

	if !fake.bootstrapped {
		t.Error("Install() did not call the launcher's bootstrap")
	}

	plistPath := filepath.Join(dir, "Library", "LaunchAgents", Label+".plist")

	content, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("reading installed plist: %v", err)
	}

	if !strings.Contains(string(content), "/Users/test/.config/mimi/config.toml") {
		t.Errorf("installed plist does not carry the config path:\n%s", content)
	}

	// The captured streams land beside log_file, never on it.
	if !strings.Contains(string(content), filepath.Join(logDir, "mimi.out.log")) {
		t.Errorf("installed plist does not carry the derived stdout path:\n%s", content)
	}

	if strings.Contains(string(content), "<string>"+logFile+"</string>") {
		t.Errorf("installed plist points a captured stream at log_file itself:\n%s", content)
	}

	// launchd opens both files at spawn and creates no directories, so a
	// missing one silently discards the console output of the very first
	// run — the startup crash these streams exist to capture.
	info, err := os.Stat(logDir)
	if err != nil {
		t.Fatalf("Install() did not create the captured-stream directory: %v", err)
	}

	if !info.IsDir() {
		t.Errorf("captured-stream path %s is not a directory", logDir)
	}
}

// TestService_Install_IsANoOpWhenTheLoadedServiceAlreadyMatches pins the
// promise that makes install safe to run at any time: with the service loaded
// and its plist already the bytes mimi would render, nothing is torn down and
// nothing is loaded again.
func TestService_Install_IsANoOpWhenTheLoadedServiceAlreadyMatches(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	logFile := filepath.Join(dir, "state", "mimi", "mimi.log")

	fake := &fakeLauncher{}
	svc := &Service{launcher: fake}

	_, err := svc.Install(testConfigPath, logFile)
	if err != nil {
		t.Fatalf("first Install() = %v, want nil", err)
	}

	// The service that install just loaded, asked to install again with the
	// same config.
	fake.loaded = true
	fake.calls = nil

	outcome, err := svc.Install(testConfigPath, logFile)
	if err != nil {
		t.Fatalf("second Install() = %v, want nil", err)
	}

	if outcome != InstallOutcomeUnchanged {
		t.Errorf("second Install() outcome = %v, want %v", outcome, InstallOutcomeUnchanged)
	}

	if len(fake.calls) != 0 {
		t.Errorf("second Install() drove launchctl: %v, want no calls", fake.calls)
	}
}

// TestService_Install_ReplacesAStalePlistAndReloadsTheService is the bug this
// idempotence exists for: the plist bakes in settings.log_file's derived
// capture paths, so a service installed before log_file moved keeps writing
// its console output to the old place until the plist is replaced. launchd
// holds the plist it was bootstrapped with, so replacing the file alone
// changes nothing — the service has to be booted out and bootstrapped again.
func TestService_Install_ReplacesAStalePlistAndReloadsTheService(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	oldLogFile := filepath.Join(dir, "old", "mimi.log")
	newLogFile := filepath.Join(dir, "new", "mimi.log")

	fake := &fakeLauncher{}
	svc := &Service{launcher: fake}

	_, err := svc.Install(testConfigPath, oldLogFile)
	if err != nil {
		t.Fatalf("first Install() = %v, want nil", err)
	}

	// The service install just loaded, with settings.log_file moved since.
	fake.loaded = true
	fake.calls = nil

	outcome, err := svc.Install(testConfigPath, newLogFile)
	if err != nil {
		t.Fatalf("second Install() = %v, want nil", err)
	}

	if outcome != InstallOutcomeReplaced {
		t.Errorf("second Install() outcome = %v, want %v", outcome, InstallOutcomeReplaced)
	}

	// Booted out first: bootstrapping over a loaded job is what makes
	// launchd re-read the plist, and it cannot while the old one is loaded.
	if got := strings.Join(fake.calls, ","); got != "bootout,bootstrap" {
		t.Errorf("second Install() calls = %v, want [bootout bootstrap]", fake.calls)
	}

	content, err := os.ReadFile(filepath.Join(dir, "Library", "LaunchAgents", Label+".plist"))
	if err != nil {
		t.Fatalf("reading installed plist: %v", err)
	}

	if !strings.Contains(string(content), filepath.Join(dir, "new", "mimi.out.log")) {
		t.Errorf("installed plist does not carry the new capture path:\n%s", content)
	}

	if strings.Contains(string(content), filepath.Join(dir, "old", "mimi.out.log")) {
		t.Errorf("installed plist still carries the old capture path:\n%s", content)
	}

	// The plist points at the new directory, so that directory has to exist
	// before launchd spawns the daemon against it.
	info, err := os.Stat(filepath.Join(dir, "new"))
	if err != nil {
		t.Fatalf("Install() did not create the new captured-stream directory: %v", err)
	}

	if !info.IsDir() {
		t.Error("the new captured-stream path is not a directory")
	}
}

// TestService_Install_FailsWhenLoadedByAnotherInstaller pins the refusal that
// survives idempotence: a loaded service with no plist of mimi's behind it is
// nix-darwin's or home-manager's, and mimi must not adopt it.
func TestService_Install_FailsWhenLoadedByAnotherInstaller(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	fake := &fakeLauncher{loaded: true}
	svc := &Service{launcher: fake}

	_, err := svc.Install(testConfigPath, filepath.Join(dir, "state", "mimi", "mimi.log"))
	if err == nil {
		t.Fatal("Install() = nil, want an error")
	}

	if derrors.GetCode(err) != derrors.CodeServiceFailed {
		t.Errorf("Install() code = %v, want %v", derrors.GetCode(err), derrors.CodeServiceFailed)
	}

	for _, tool := range []string{"nix-darwin", "home-manager"} {
		if !strings.Contains(err.Error(), tool) {
			t.Errorf("Install() error %q does not name %s", err, tool)
		}
	}

	if len(fake.calls) != 0 {
		t.Errorf("Install() drove launchctl: %v, want no calls", fake.calls)
	}
}

// TestService_Install_FailsWhenThePlistIsNotAFileMimiWrote covers the plist
// home-manager installs: a symlink into the Nix store. Replacing it would
// break its link and lose to the next rebuild anyway.
func TestService_Install_FailsWhenThePlistIsNotAFileMimiWrote(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	agentsDir := filepath.Join(dir, "Library", "LaunchAgents")

	err := os.MkdirAll(agentsDir, 0o755)
	if err != nil {
		t.Fatalf("creating LaunchAgents dir: %v", err)
	}

	storePlist := filepath.Join(dir, "nix-store-mimi.plist")

	err = os.WriteFile(storePlist, []byte("someone else's plist"), 0o644)
	if err != nil {
		t.Fatalf("seeding store plist: %v", err)
	}

	plistPath := filepath.Join(agentsDir, Label+".plist")

	err = os.Symlink(storePlist, plistPath)
	if err != nil {
		t.Fatalf("seeding plist symlink: %v", err)
	}

	fake := &fakeLauncher{}
	svc := &Service{launcher: fake}

	_, err = svc.Install(testConfigPath, filepath.Join(dir, "state", "mimi", "mimi.log"))
	if err == nil {
		t.Fatal("Install() = nil, want an error")
	}

	if derrors.GetCode(err) != derrors.CodeServiceFailed {
		t.Errorf("Install() code = %v, want %v", derrors.GetCode(err), derrors.CodeServiceFailed)
	}

	for _, tool := range []string{"nix-darwin", "home-manager"} {
		if !strings.Contains(err.Error(), tool) {
			t.Errorf("Install() error %q does not name %s", err, tool)
		}
	}

	if len(fake.calls) != 0 {
		t.Errorf("Install() drove launchctl: %v, want no calls", fake.calls)
	}

	// The symlink is still a symlink, still pointing where it did.
	info, err := os.Lstat(plistPath)
	if err != nil {
		t.Fatalf("Install() removed the foreign plist: %v", err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("Install() replaced the foreign plist symlink with a file")
	}
}

// TestService_Install_LoadsAPlistLeftBehindByAPreviousInstall covers the
// half-installed machine: an uninstall that removed nothing, or a service
// booted out by hand. The plist is rewritten from the current config and
// bootstrapped, rather than the install refusing because a file is in the way.
func TestService_Install_LoadsAPlistLeftBehindByAPreviousInstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	agentsDir := filepath.Join(dir, "Library", "LaunchAgents")

	err := os.MkdirAll(agentsDir, 0o755)
	if err != nil {
		t.Fatalf("creating LaunchAgents dir: %v", err)
	}

	plistPath := filepath.Join(agentsDir, Label+".plist")

	err = os.WriteFile(plistPath, []byte("a stale plist"), 0o644)
	if err != nil {
		t.Fatalf("seeding existing plist: %v", err)
	}

	fake := &fakeLauncher{}
	svc := &Service{launcher: fake}

	outcome, err := svc.Install(testConfigPath, filepath.Join(dir, "state", "mimi", "mimi.log"))
	if err != nil {
		t.Fatalf("Install() = %v, want nil", err)
	}

	if outcome != InstallOutcomeInstalled {
		t.Errorf("Install() outcome = %v, want %v", outcome, InstallOutcomeInstalled)
	}

	// Nothing was loaded, so there was nothing to boot out.
	if diff := strings.Join(fake.calls, ","); diff != "bootstrap" {
		t.Errorf("Install() calls = %v, want [bootstrap]", fake.calls)
	}

	content, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("reading installed plist: %v", err)
	}

	if !strings.Contains(string(content), testConfigPath) {
		t.Errorf("Install() left the stale plist in place:\n%s", content)
	}
}

// TestService_Install_NeverLeavesAPartialPlist pins the atomic replace. The
// plist goes to a temporary file in the same directory and is renamed over the
// target, so a replace that fails leaves the previous plist whole and readable
// — launchd loads it at every login, and a truncated one is a service that
// stops coming back. The temporary file is not litter left in LaunchAgents
// either way.
func TestService_Install_NeverLeavesAPartialPlist(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory permissions this test denies writes with")
	}

	dir := t.TempDir()
	t.Setenv("HOME", dir)

	fake := &fakeLauncher{}
	svc := &Service{launcher: fake}

	_, err := svc.Install(testConfigPath, filepath.Join(dir, "old", "mimi.log"))
	if err != nil {
		t.Fatalf("first Install() = %v, want nil", err)
	}

	agentsDir := filepath.Join(dir, "Library", "LaunchAgents")
	plistPath := filepath.Join(agentsDir, Label+".plist")

	installed, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("reading installed plist: %v", err)
	}

	assertOnlyThePlist(t, agentsDir)

	// A LaunchAgents directory no new file can be created in — the shape any
	// interruption of the replace takes, seen from the target file.
	err = os.Chmod(agentsDir, 0o555)
	if err != nil {
		t.Fatalf("making LaunchAgents read-only: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(agentsDir, 0o755) })

	fake.loaded = true

	_, err = svc.Install(testConfigPath, filepath.Join(dir, "new", "mimi.log"))
	if err == nil {
		t.Fatal("second Install() = nil, want an error")
	}

	if derrors.GetCode(err) != derrors.CodeServiceFailed {
		t.Errorf(
			"second Install() code = %v, want %v",
			derrors.GetCode(err),
			derrors.CodeServiceFailed,
		)
	}

	after, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("reading the plist after the failed replace: %v", err)
	}

	if string(after) != string(installed) {
		t.Errorf(
			"a failed replace changed the installed plist:\ngot\n%s\nwant\n%s",
			after,
			installed,
		)
	}

	assertOnlyThePlist(t, agentsDir)
}

// TestService_Install_KeepsTheStalePlistWhenTheServiceCannotBeUnloaded pins
// the order of the replace. Writing the plist before unloading the service
// would leave, on a failed unload, a new plist on disk in front of a service
// still running the old one — and every later install would find the file it
// wanted and report the service up to date, forever. That is the exact
// "stale plist believed current" failure idempotence exists to remove, so the
// failed install has to leave the old plist behind for the next one to find.
func TestService_Install_KeepsTheStalePlistWhenTheServiceCannotBeUnloaded(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	fake := &fakeLauncher{}
	svc := &Service{launcher: fake}

	_, err := svc.Install(testConfigPath, filepath.Join(dir, "old", "mimi.log"))
	if err != nil {
		t.Fatalf("first Install() = %v, want nil", err)
	}

	plistPath := filepath.Join(dir, "Library", "LaunchAgents", Label+".plist")

	stale, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("reading installed plist: %v", err)
	}

	fake.loaded = true
	fake.bootoutErr = derrors.New(derrors.CodeServiceFailed, "launchd said no")
	fake.calls = nil

	newLogFile := filepath.Join(dir, "new", "mimi.log")

	_, err = svc.Install(testConfigPath, newLogFile)
	if err == nil {
		t.Fatal("Install() = nil, want the unload failure")
	}

	if derrors.GetCode(err) != derrors.CodeServiceFailed {
		t.Errorf("Install() code = %v, want %v", derrors.GetCode(err), derrors.CodeServiceFailed)
	}

	// Nothing was written, so nothing was bootstrapped over the old service.
	if got := strings.Join(fake.calls, ","); got != "bootout" {
		t.Errorf("Install() calls = %v, want [bootout]", fake.calls)
	}

	after, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("reading the plist after the failed unload: %v", err)
	}

	if string(after) != string(stale) {
		t.Errorf("a failed unload replaced the plist:\ngot\n%s\nwant\n%s", after, stale)
	}

	// The next install still sees the disagreement, rather than a plist that
	// already matches in front of a service that never got it.
	fake.bootoutErr = nil
	fake.calls = nil

	outcome, err := svc.Install(testConfigPath, newLogFile)
	if err != nil {
		t.Fatalf("retried Install() = %v, want nil", err)
	}

	if outcome != InstallOutcomeReplaced {
		t.Errorf("retried Install() outcome = %v, want %v", outcome, InstallOutcomeReplaced)
	}
}

// TestService_Install_DoesNotUseTheSystemTempDirectory pins the "same
// directory" half of the atomic write. A temporary file anywhere else risks a
// rename across filesystems, which is not atomic and can fail outright — and
// a TMPDIR that does not exist is how a test can tell the two apart.
func TestService_Install_DoesNotUseTheSystemTempDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("TMPDIR", filepath.Join(dir, "no-such-temp-dir"))

	fake := &fakeLauncher{}
	svc := &Service{launcher: fake}

	_, err := svc.Install(testConfigPath, filepath.Join(dir, "state", "mimi", "mimi.log"))
	if err != nil {
		t.Fatalf("Install() = %v, want nil (the plist never goes via TMPDIR)", err)
	}

	agentsDir := filepath.Join(dir, "Library", "LaunchAgents")

	_, err = os.Stat(filepath.Join(agentsDir, Label+".plist"))
	if err != nil {
		t.Fatalf("Install() wrote no plist: %v", err)
	}

	assertOnlyThePlist(t, agentsDir)
}

// assertOnlyThePlist fails unless dir holds mimi's plist and nothing else, so
// that a temporary file left behind by the atomic write is caught wherever it
// is checked.
func assertOnlyThePlist(t *testing.T, dir string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading LaunchAgents: %v", err)
	}

	if len(entries) != 1 || entries[0].Name() != Label+".plist" {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}

		t.Errorf("LaunchAgents holds %v, want only %s", names, Label+".plist")
	}
}

// TestService_Install_FailsWhenTheCapturedStreamDirectoryCannotBeCreated
// pins that an unusable log_file directory is reported rather than swallowed:
// launchd would otherwise load a service whose console output goes nowhere.
func TestService_Install_FailsWhenTheCapturedStreamDirectoryCannotBeCreated(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// A regular file where the log directory would have to be.
	blocker := filepath.Join(dir, "state")

	err := os.WriteFile(blocker, []byte("not a directory"), 0o644)
	if err != nil {
		t.Fatalf("seeding blocking file: %v", err)
	}

	fake := &fakeLauncher{}
	svc := &Service{launcher: fake}

	_, err = svc.Install(testConfigPath, filepath.Join(blocker, "mimi", "mimi.log"))
	if err == nil {
		t.Fatal("Install() = nil, want an error")
	}

	if derrors.GetCode(err) != derrors.CodeServiceFailed {
		t.Errorf("Install() code = %v, want %v", derrors.GetCode(err), derrors.CodeServiceFailed)
	}

	if fake.bootstrapped {
		t.Error("Install() bootstrapped despite an unusable log directory")
	}
}
