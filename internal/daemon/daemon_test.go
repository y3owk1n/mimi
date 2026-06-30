//nolint:testpackage
package daemon

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/config"
)

const hookRunEcho = "echo"

func TestHasWindowEvents(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected bool
	}{
		{
			name:     "empty config",
			cfg:      &config.Config{},
			expected: false,
		},
		{
			name: "window focus only",
			cfg: &config.Config{
				Hooks: config.HooksConfig{
					WindowFocus: []config.HookEntry{{Run: hookRunEcho}},
				},
			},
			expected: true,
		},
		{
			name: "workspace only",
			cfg: &config.Config{
				Hooks: config.HooksConfig{
					WorkspaceChanged: []config.HookEntry{{Run: hookRunEcho}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasWindowEvents(tt.cfg)
			if result != tt.expected {
				t.Errorf("hasWindowEvents() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGetObserverConfig(t *testing.T) {
	cfg := &config.Config{
		Hooks: config.HooksConfig{
			WindowFocus:      []config.HookEntry{{Run: hookRunEcho}},
			WorkspaceChanged: []config.HookEntry{{Run: hookRunEcho}},
		},
	}

	obs := getObserverConfig(cfg)

	if !obs.AppLifecycle {
		t.Error("expected AppLifecycle to be true when window hooks are configured")
	}

	if !obs.Workspace {
		t.Error("expected Workspace to be true when workspace hooks are configured")
	}

	emptyObs := getObserverConfig(&config.Config{})
	if emptyObs.AppLifecycle || emptyObs.Workspace {
		t.Errorf("expected all observers disabled on empty config, got: %+v", emptyObs)
	}

	workspaceOnlyObs := getObserverConfig(&config.Config{
		Hooks: config.HooksConfig{
			WorkspaceChanged: []config.HookEntry{{Run: hookRunEcho}},
		},
	})
	if workspaceOnlyObs.AppLifecycle {
		t.Error("expected AppLifecycle disabled for workspace-only config")
	}

	if !workspaceOnlyObs.Workspace {
		t.Error("expected Workspace enabled for workspace-only config")
	}
}
