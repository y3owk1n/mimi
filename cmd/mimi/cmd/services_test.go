//nolint:testpackage
package cmd

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/y3owk1n/mimi/internal/service"
)

// TestCLIState_PlistSettings covers what `mimi services install` hands the
// plist renderer: settings.log_file, which decides whether the installed
// service captures stdout beside the log file or in /tmp (issue #99), and
// settings.service_path, which is the PATH that service runs its hooks with
// (issue #156). Either is "" whenever there is none to hand over, and both are
// when the config cannot be read at all.
func TestCLIState_PlistSettings(t *testing.T) {
	tests := []struct {
		name string
		// body is the config to write; an empty body means write no config
		// file at all, so config.Load fails.
		body            string
		wantLogFile     string
		wantServicePath string
	}{
		{
			name: "both set",
			body: fmt.Sprintf(
				"[settings]\nlog_file = %q\nservice_path = %q\n",
				"/Users/test/.local/state/mimi/mimi.log",
				"/Users/test/.local/bin:/usr/bin",
			),
			wantLogFile:     "/Users/test/.local/state/mimi/mimi.log",
			wantServicePath: "/Users/test/.local/bin:/usr/bin",
		},
		{
			name:            "both unset",
			body:            "[settings]\nlog_level = \"debug\"\n",
			wantLogFile:     "",
			wantServicePath: "",
		},
		{
			name:            "config unreadable",
			body:            "",
			wantLogFile:     "",
			wantServicePath: "",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.toml")
			if testCase.body != "" {
				writeConfigFile(t, configPath, testCase.body)
			}

			state := &cliState{configPath: configPath}

			logFile, servicePath := state.plistSettings()
			if logFile != testCase.wantLogFile {
				t.Errorf("plistSettings() log file = %q, want %q", logFile, testCase.wantLogFile)
			}

			if servicePath != testCase.wantServicePath {
				t.Errorf(
					"plistSettings() service path = %q, want %q",
					servicePath,
					testCase.wantServicePath,
				)
			}
		})
	}
}

// TestFormatInstallOutcome pins that each of the three things `mimi services
// install` can do gets its own line. They are distinguishable on purpose: a
// replace and a no-op both exit 0, and only the wording separates a service
// that was just brought in line with the config from one that never needed it.
func TestFormatInstallOutcome(t *testing.T) {
	tests := []struct {
		name    string
		outcome service.InstallOutcome
		want    string
	}{
		{
			name:    "installed",
			outcome: service.InstallOutcomeInstalled,
			want:    "Service installed and loaded successfully",
		},
		{
			name:    "replaced",
			outcome: service.InstallOutcomeReplaced,
			want:    "Service plist updated and service reloaded",
		},
		{
			name:    "unchanged",
			outcome: service.InstallOutcomeUnchanged,
			want:    "Service already up to date",
		},
		{
			name:    "an outcome with no line of its own",
			outcome: service.InstallOutcome(0),
			want:    "Service install completed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatInstallOutcome(tt.outcome)
			if got != tt.want {
				t.Errorf("formatInstallOutcome(%v) = %q, want %q", tt.outcome, got, tt.want)
			}
		})
	}
}

// TestFormatStatus pins the line each shape of a status prints. The wording is
// the whole feature: the installed plist relaunches a crashing daemon forever,
// so "loaded" said on its own is the answer that hides the failure this
// command is run to find.
func TestFormatStatus(t *testing.T) {
	tests := []struct {
		name   string
		status service.Status
		want   string
	}{
		{
			name: "loaded and running",
			status: service.Status{
				State: service.LoadStateLoaded,
				PID:   service.OptionalInt{Value: 1478, Known: true},
			},
			want: "Service loaded and running (pid 1478)",
		},
		{
			name: "loaded but respawning after a crash",
			status: service.Status{
				State:          service.LoadStateLoaded,
				LastExitStatus: service.OptionalInt{Value: 1, Known: true},
			},
			want: "Service loaded but not running (last exit status 1)",
		},
		{
			// A clean exit is still not running, and launchd will bring it
			// back: the number is what separates it from a crash.
			name: "loaded, exited cleanly",
			status: service.Status{
				State:          service.LoadStateLoaded,
				LastExitStatus: service.OptionalInt{Value: 0, Known: true},
			},
			want: "Service loaded but not running (last exit status 0)",
		},
		{
			// A job that is running now has a last exit status from before it
			// was restarted. The pid is the fact worth printing.
			name: "loaded and running, having exited before",
			status: service.Status{
				State:          service.LoadStateLoaded,
				PID:            service.OptionalInt{Value: 1478, Known: true},
				LastExitStatus: service.OptionalInt{Value: 1, Known: true},
			},
			want: "Service loaded and running (pid 1478)",
		},
		{
			// launchd's description could not be read, so the command falls
			// back to everything it has ever said.
			name:   "loaded, with nothing else known",
			status: service.Status{State: service.LoadStateLoaded},
			want:   "Service loaded",
		},
		{
			name:   "not loaded",
			status: service.Status{State: service.LoadStateNotLoaded},
			want:   "Service not loaded",
		},
		{
			// The state that used to print as "Service not loaded": launchctl
			// could not be run, so nothing here knows whether the service is
			// up. Saying so names the thing to fix, where the old line sent a
			// user to reinstall a service that may be running fine.
			name:   "launchctl could not be asked",
			status: service.Status{State: service.LoadStateUnknown},
			want:   "Service state unknown: launchctl could not be run",
		},
		{
			// The captured streams are files on disk, so they are still worth
			// printing under a load state nobody could read.
			name: "launchctl could not be asked, with a captured stream",
			status: service.Status{
				State: service.LoadStateUnknown,
				CapturedStderr: service.CapturedLog{
					Path:    "/tmp/mimi.err.log",
					Size:    12,
					Present: true,
				},
			},
			want: "Service state unknown: launchctl could not be run\n" +
				"Captured stderr: /tmp/mimi.err.log (12 B)",
		},
		{
			// Nothing rotates the captured streams, so the daemon empties them
			// once per start and a size is one run's console output. Printing
			// it next to the path is what turns "the stderr says why" into
			// something a user can act on without going looking first.
			name: "loaded and running, with both captured streams",
			status: service.Status{
				State: service.LoadStateLoaded,
				PID:   service.OptionalInt{Value: 1478, Known: true},
				CapturedStdout: service.CapturedLog{
					Path:    "/Users/test/.local/state/mimi/mimi.out.log",
					Size:    2048,
					Present: true,
				},
				CapturedStderr: service.CapturedLog{
					Path:    "/Users/test/.local/state/mimi/mimi.err.log",
					Size:    0,
					Present: true,
				},
			},
			want: "Service loaded and running (pid 1478)\n" +
				"Captured stdout: /Users/test/.local/state/mimi/mimi.out.log (2.0 KB)\n" +
				"Captured stderr: /Users/test/.local/state/mimi/mimi.err.log (0 B)",
		},
		{
			// launchd creates both files when it first spawns the daemon, so a
			// missing one is a service that has never run — a different thing
			// from an empty one, and said differently.
			name: "loaded, with a captured stream that does not exist yet",
			status: service.Status{
				State: service.LoadStateLoaded,
				CapturedStdout: service.CapturedLog{
					Path: "/Users/test/.local/state/mimi/mimi.out.log",
				},
			},
			want: "Service loaded\n" +
				"Captured stdout: /Users/test/.local/state/mimi/mimi.out.log (not created yet)",
		},
		{
			// The files outlive the service that wrote them, and an unloaded
			// service is one of the states in which their contents matter most.
			name: "not loaded, with the plist still naming its captured streams",
			status: service.Status{
				State: service.LoadStateNotLoaded,
				CapturedStderr: service.CapturedLog{
					Path:    "/tmp/mimi.err.log",
					Size:    1288490189,
					Present: true,
				},
			},
			want: "Service not loaded\nCaptured stderr: /tmp/mimi.err.log (1.2 GB)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatStatus(tt.status)
			if got != tt.want {
				t.Errorf("formatStatus(%+v) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}
