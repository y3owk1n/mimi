package doctor_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/doctor"
	"github.com/y3owk1n/mimi/internal/service"
)

var errBadTOML = errors.New("line 3: bad toml")

func statusOf(t *testing.T, checks []doctor.Check, name string) doctor.Check {
	t.Helper()

	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}

	t.Fatalf("no check named %q", name)

	return doctor.Check{}
}

func healthy() doctor.Facts {
	return doctor.Facts{
		Config:        &config.Config{},
		Accessibility: true,
		PIDPath:       "/tmp/mimi.pid",
		SocketPath:    "/tmp/mimi.sock",
		PID:           42,
		PIDFound:      true,
		Alive:         true,
		SocketPresent: true,
		CLIVersion:    "v1.2.3",
		DaemonVersion: "v1.2.3",
		Service: service.Status{
			State: service.LoadStateLoaded,
			PID:   service.OptionalInt{Value: 42, Known: true},
		},
		LogFile:        "/tmp/mimi.log",
		LogDirWritable: true,
	}
}

func TestAssess_HealthyInstallPasses(t *testing.T) {
	checks := doctor.Assess(healthy())

	if doctor.Failed(checks) {
		t.Fatalf("healthy facts failed:\n%s", doctor.Format(checks))
	}

	for _, check := range checks {
		if check.Status != doctor.Pass {
			t.Errorf("%s: got %s, want ok", check.Name, check.Status)
		}
	}
}

func TestAssess_StalePIDFileFailsWithTheFix(t *testing.T) {
	facts := healthy()
	facts.Alive = false

	check := statusOf(t, doctor.Assess(facts), "daemon")
	if check.Status != doctor.Fail || !strings.Contains(check.Detail, "stale PID file") {
		t.Fatalf("got %+v", check)
	}

	if check.Fix == "" {
		t.Fatal("a failure carries no fix")
	}
}

func TestAssess_NoDaemonSkipsTheSocket(t *testing.T) {
	facts := healthy()
	facts.PIDFound = false

	checks := doctor.Assess(facts)
	if statusOf(t, checks, "daemon").Status != doctor.Warn {
		t.Fatal("a daemon not running is a warning, not a failure")
	}

	if statusOf(t, checks, "socket").Status != doctor.Skip {
		t.Fatal("the socket check ran with no daemon to reach")
	}

	if statusOf(t, checks, "daemon build").Status != doctor.Skip {
		t.Fatal("the build check ran with no daemon to ask")
	}
}

func TestAssess_ADaemonOfAnotherBuildFails(t *testing.T) {
	facts := healthy()
	facts.DaemonVersion = "v1.2.2"

	check := statusOf(t, doctor.Assess(facts), "daemon build")
	if check.Status != doctor.Fail || !strings.Contains(check.Fix, "restart the daemon") {
		t.Fatalf("got %+v", check)
	}

	facts = healthy()
	facts.DaemonVersion, facts.ProbeErr = "", errBadTOML

	if statusOf(t, doctor.Assess(facts), "daemon build").Status != doctor.Fail {
		t.Fatal("a daemon that refused the request passed")
	}
}

func TestAssess_BrokenConfigFails(t *testing.T) {
	facts := healthy()
	facts.Config, facts.ConfigErr = nil, errBadTOML

	checks := doctor.Assess(facts)
	if statusOf(t, checks, "config").Status != doctor.Fail {
		t.Fatal("a config that does not load passed")
	}

	if !doctor.Failed(checks) {
		t.Fatal("Failed missed the failed check")
	}
}

func TestAssess_ServiceInCrashLoopPointsAtStderr(t *testing.T) {
	facts := healthy()
	facts.Service = service.Status{
		State:          service.LoadStateLoaded,
		LastExitStatus: service.OptionalInt{Value: 1, Known: true},
		CapturedStderr: service.CapturedLog{Path: "/tmp/mimi.err.log"},
	}

	check := statusOf(t, doctor.Assess(facts), "service")
	if check.Status != doctor.Fail || !strings.Contains(check.Fix, "/tmp/mimi.err.log") {
		t.Fatalf("got %+v", check)
	}
}

func TestAssess_NamesTheAgentRunningADaemonThatIsNotMimisService(t *testing.T) {
	facts := healthy()
	facts.Service = service.Status{State: service.LoadStateNotLoaded}
	facts.ForeignAgent = "org.nix-community.home.mimi"

	check := statusOf(t, doctor.Assess(facts), "service")
	if check.Status != doctor.Skip ||
		!strings.Contains(check.Detail, "org.nix-community.home.mimi") {
		t.Fatalf("got %+v", check)
	}
}

func TestMissingHookCommands_ResolvesAgainstTheServicePath(t *testing.T) {
	bin := t.TempDir()

	err := os.WriteFile(filepath.Join(bin, "sketchybar"), []byte("#!/bin/sh\n"), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Hooks.AppActivate = []config.HookEntry{
		{Run: "sketchybar --trigger front_app"},
		{Run: "FOO=1 notaprogram $mimi_APP_NAME"},
		{Run: "/usr/local/bin/anything"},
		{Run: "echo hi"},
	}
	cfg.Hooks.WorkspaceChanged = []config.HookEntry{{Run: "notaprogram again"}}

	missing := doctor.MissingHookCommands(cfg, bin+":/nonexistent")
	if len(missing) != 1 || missing[0] != "notaprogram" {
		t.Fatalf("got %v, want [notaprogram]", missing)
	}
}

func TestFormat_PrintsAFixUnderAFailure(t *testing.T) {
	out := doctor.Format([]doctor.Check{
		{Name: "config", Status: doctor.Pass, Detail: "/x/config.toml"},
		{Name: "accessibility", Status: doctor.Fail, Detail: "not granted", Fix: "grant it"},
	})

	want := "ok    config         /x/config.toml\nFAIL  accessibility  not granted\n      fix: grant it\n"
	if out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}
