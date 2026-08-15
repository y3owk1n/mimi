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

// fakeLauncher is a launcher made of plain values: every call it sees lands
// in a field a test can assert against, and every call it returns is a field
// a test can set up in advance.
type fakeLauncher struct {
	loaded bool

	bootstrapErr error
	bootstrapped bool
	bootoutErr   error
	booted       bool
	startErr     error
	started      bool
	stopErr      error
	stopped      bool
}

func (f *fakeLauncher) list(_ context.Context, _ string) error {
	if f.loaded {
		return nil
	}

	return derrors.New(derrors.CodeServiceFailed, "not loaded")
}

func (f *fakeLauncher) bootstrap(_ context.Context, _, _ string) error {
	f.bootstrapped = true

	return f.bootstrapErr
}

func (f *fakeLauncher) bootout(_ context.Context, _ string) error {
	f.booted = true

	return f.bootoutErr
}

func (f *fakeLauncher) start(_ context.Context, _ string) error {
	f.started = true

	return f.startErr
}

func (f *fakeLauncher) stop(_ context.Context, _ string) error {
	f.stopped = true

	return f.stopErr
}

func TestService_Status_ReportsWhatTheLauncherSees(t *testing.T) {
	tests := []struct {
		name   string
		loaded bool
		want   Status
	}{
		{name: "loaded", loaded: true, want: Status{Loaded: true}},
		{name: "not loaded", loaded: false, want: Status{Loaded: false}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			svc := &Service{launcher: &fakeLauncher{loaded: testCase.loaded}}

			got := svc.Status()
			if got != testCase.want {
				t.Errorf("Status() = %+v, want %+v", got, testCase.want)
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

	err := svc.Install("/Users/test/.config/mimi/config.toml", logFile)
	if err != nil {
		t.Fatalf("Install() = %v, want nil", err)
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

func TestService_Install_FailsWhenAlreadyLoaded(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	fake := &fakeLauncher{loaded: true}
	svc := &Service{launcher: fake}

	err := svc.Install(
		"/Users/test/.config/mimi/config.toml",
		filepath.Join(dir, "state", "mimi", "mimi.log"),
	)
	if err == nil {
		t.Fatal("Install() = nil, want an error")
	}

	if derrors.GetCode(err) != derrors.CodeServiceFailed {
		t.Errorf("Install() code = %v, want %v", derrors.GetCode(err), derrors.CodeServiceFailed)
	}

	if fake.bootstrapped {
		t.Error("Install() bootstrapped an already-loaded service")
	}
}

func TestService_Install_FailsWhenThePlistAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	agentsDir := filepath.Join(dir, "Library", "LaunchAgents")

	err := os.MkdirAll(agentsDir, 0o755)
	if err != nil {
		t.Fatalf("creating LaunchAgents dir: %v", err)
	}

	err = os.WriteFile(filepath.Join(agentsDir, Label+".plist"), []byte("existing"), 0o644)
	if err != nil {
		t.Fatalf("seeding existing plist: %v", err)
	}

	fake := &fakeLauncher{}
	svc := &Service{launcher: fake}

	err = svc.Install(
		"/Users/test/.config/mimi/config.toml",
		filepath.Join(dir, "state", "mimi", "mimi.log"),
	)
	if err == nil {
		t.Fatal("Install() = nil, want an error")
	}

	if derrors.GetCode(err) != derrors.CodeServiceFailed {
		t.Errorf("Install() code = %v, want %v", derrors.GetCode(err), derrors.CodeServiceFailed)
	}

	if fake.bootstrapped {
		t.Error("Install() bootstrapped despite a pre-existing plist")
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

	err = svc.Install(
		"/Users/test/.config/mimi/config.toml",
		filepath.Join(blocker, "mimi", "mimi.log"),
	)
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
