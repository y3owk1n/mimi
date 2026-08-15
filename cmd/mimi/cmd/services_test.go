//nolint:testpackage
package cmd

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/y3owk1n/mimi/internal/service"
)

// TestCLIState_LogFilePath covers what `mimi services install` hands the plist
// renderer: the configured settings.log_file, and "" whenever there is none to
// hand over — which decides whether the installed service captures stdout
// beside the log file or in /tmp. See issue #99.
func TestCLIState_LogFilePath(t *testing.T) {
	tests := []struct {
		name string
		// body is the config to write; an empty body means write no config
		// file at all, so config.Load fails.
		body string
		want string
	}{
		{
			name: "log_file set",
			body: fmt.Sprintf(
				"[settings]\nlog_file = %q\n",
				"/Users/test/.local/state/mimi/mimi.log",
			),
			want: "/Users/test/.local/state/mimi/mimi.log",
		},
		{
			name: "log_file unset",
			body: "[settings]\nlog_level = \"debug\"\n",
			want: "",
		},
		{
			name: "config unreadable",
			body: "",
			want: "",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.toml")
			if testCase.body != "" {
				writeConfigFile(t, configPath, testCase.body)
			}

			state := &cliState{configPath: configPath}
			if got := state.logFilePath(); got != testCase.want {
				t.Errorf("logFilePath() = %q, want %q", got, testCase.want)
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

func TestFormatStatus(t *testing.T) {
	tests := []struct {
		name   string
		status service.Status
		want   string
	}{
		{name: "loaded", status: service.Status{Loaded: true}, want: "Service loaded"},
		{name: "not loaded", status: service.Status{Loaded: false}, want: "Service not loaded"},
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
