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
