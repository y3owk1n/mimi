package config //nolint:testpackage // exercises the unexported classification walk

import (
	"reflect"
	"testing"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// runningConfig is the config a reload is compared against: every field set to
// a value distinguishable from the zero value, so a test that changes one
// field changes it to something the comparison can actually see.
func runningConfig() *Config {
	return &Config{
		Settings: SettingsConfig{
			LogFile:          "/tmp/mimi.log",
			LogLevel:         "info",
			LogFormat:        "text",
			HookTimeoutSecs:  10,
			HookShell:        "/bin/sh",
			MaxHookWorkers:   4,
			PIDFile:          "/tmp/mimi.pid",
			SocketFile:       "/tmp/mimi.sock",
			ResizeDebounceMS: 250,
			ServicePath:      "/usr/bin:/bin",
		},
		Systray: SystrayConfig{Enabled: true, ShowWorkspaceNumber: true},
	}
}

// TestClassifyFields_RejectsAnUnclassifiedField pins the guard that stands in
// for a compile-time check: a config field nobody classified must stop the
// package rather than be quietly reported as reloadable. The real Config type
// goes through the same walk at package init, so the failure a new field
// produces is this error, raised as a panic before any test runs.
func TestClassifyFields_RejectsAnUnclassifiedField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		structType reflect.Type
	}{
		{
			name: "a leaf field with no reload tag",
			structType: reflect.TypeOf(struct {
				Classified   string `reload:"reloadable" toml:"classified"`
				Unclassified string `toml:"unclassified"`
			}{}),
		},
		{
			name: "a reload tag that names no classification",
			structType: reflect.TypeOf(struct {
				Nonsense string `reload:"sometimes" toml:"nonsense"`
			}{}),
		},
		{
			name: "a section with no reload tag",
			structType: reflect.TypeOf(struct {
				Section struct {
					Classified string `reload:"reloadable" toml:"classified"`
				} `toml:"section"`
			}{}),
		},
		{
			name: "a leaf inside a per-field section",
			structType: reflect.TypeOf(struct {
				Section struct {
					Unclassified string `toml:"unclassified"`
				} `reload:"per-field" toml:"section"`
			}{}),
		},
		{
			name: "a leaf tagged as a section",
			structType: reflect.TypeOf(struct {
				NotASection string `reload:"per-field" toml:"not_a_section"`
			}{}),
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := classifyFields(testCase.structType, "", nil)
			if err == nil {
				t.Fatal("expected an error for an unclassified config field, got nil")
			}

			if !derrors.IsCode(err, derrors.CodeInternal) {
				t.Errorf("expected CodeInternal, got %v", derrors.GetCode(err))
			}
		})
	}
}

// TestClassifyFields_AcceptsEveryClassification is the other half of the guard
// above: the tags a field may carry, pinned as a set so that adding a third
// leaf classification means teaching the walk rather than working around it. A
// field tagged reinstall-only must classify, not error
// (docs/adr/0003-a-setting-the-daemon-never-reads-is-reinstall-only.md).
func TestClassifyFields_AcceptsEveryClassification(t *testing.T) {
	t.Parallel()

	fields, err := classifyFields(reflect.TypeOf(struct {
		Reloads   string `reload:"reloadable"     toml:"reloads"`
		Restarts  string `reload:"restart-only"   toml:"restarts"`
		Reinstall string `reload:"reinstall-only" toml:"reinstall"`
	}{}), "", nil)
	if err != nil {
		t.Fatalf("classifyFields() = %v, want nil", err)
	}

	want := []reloadability{reloadable, restartOnly, reinstallOnly}

	got := make([]reloadability, 0, len(fields))
	for _, field := range fields {
		got = append(got, field.kind)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("classifyFields() classifications = %v, want %v", got, want)
	}
}

func TestRestartOnlyChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(*Config)
		want   []string
	}{
		{
			name:   "an unchanged config needs no restart",
			change: func(*Config) {},
			want:   nil,
		},
		{
			name:   "a reloadable setting needs no restart",
			change: func(c *Config) { c.Settings.HookShell = "/bin/bash" },
			want:   nil,
		},
		{
			name:   "reloadable hooks need no restart",
			change: func(c *Config) { c.Hooks.WindowFocus = []HookEntry{{Run: "echo hi"}} },
			want:   nil,
		},
		{
			name:   "a changed log level is restart-only",
			change: func(c *Config) { c.Settings.LogLevel = "debug" },
			want:   []string{"settings.log_level"},
		},
		{
			name:   "a changed log format is restart-only",
			change: func(c *Config) { c.Settings.LogFormat = "json" },
			want:   []string{"settings.log_format"},
		},
		{
			name:   "a changed hook worker limit is restart-only",
			change: func(c *Config) { c.Settings.MaxHookWorkers = 8 },
			want:   []string{"settings.max_hook_workers"},
		},
		{
			name:   "a changed systray field is restart-only",
			change: func(c *Config) { c.Systray.ShowWorkspaceNumber = false },
			want:   []string{"systray.show_workspace_number"},
		},
		{
			// A reinstall-only setting is not restart-only: restarting the
			// daemon runs it against the same plist, so reporting a restart
			// here would be the wrong instruction, confidently given.
			name:   "a reinstall-only setting needs no restart",
			change: func(c *Config) { c.Settings.ServicePath = "/opt/homebrew/bin:/usr/bin" },
			want:   nil,
		},
		{
			name: "several restart-only changes are all named, in declaration order",
			change: func(c *Config) {
				c.Systray.Enabled = false
				c.Settings.SocketFile = "/tmp/other.sock"
				c.Settings.LogFile = "/tmp/other.log"
			},
			want: []string{
				"settings.log_file",
				"settings.socket_file",
				"systray.enabled",
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			next := runningConfig()
			testCase.change(next)

			got := RestartOnlyChanges(runningConfig(), next)
			if !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("RestartOnlyChanges() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestReinstallOnlyChanges covers the classification the daemon never reads:
// a change to one of these takes effect when the service is installed again,
// so it is reported separately from the settings a restart would pick up.
func TestReinstallOnlyChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(*Config)
		want   []string
	}{
		{
			name:   "an unchanged config needs no reinstall",
			change: func(*Config) {},
			want:   nil,
		},
		{
			name:   "a reloadable setting needs no reinstall",
			change: func(c *Config) { c.Settings.HookShell = "/bin/bash" },
			want:   nil,
		},
		{
			name:   "a restart-only setting needs no reinstall",
			change: func(c *Config) { c.Settings.LogLevel = "debug" },
			want:   nil,
		},
		{
			name:   "a changed service path is reinstall-only",
			change: func(c *Config) { c.Settings.ServicePath = "/opt/homebrew/bin:/usr/bin" },
			want:   []string{"settings.service_path"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			next := runningConfig()
			testCase.change(next)

			got := ReinstallOnlyChanges(runningConfig(), next)
			if !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("ReinstallOnlyChanges() = %v, want %v", got, testCase.want)
			}
		})
	}
}
