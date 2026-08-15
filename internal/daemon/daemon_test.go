//nolint:testpackage
package daemon

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/config"
)

const (
	hookRunEcho             = "echo"
	testCaseNameEmptyConfig = "empty config"
)

func TestHasWindowEvents(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected bool
	}{
		{
			name:     testCaseNameEmptyConfig,
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

func TestHasAppEvents(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected bool
	}{
		{
			name:     testCaseNameEmptyConfig,
			cfg:      &config.Config{},
			expected: false,
		},
		{
			name: "app activate only",
			cfg: &config.Config{
				Hooks: config.HooksConfig{
					AppActivate: []config.HookEntry{{Run: hookRunEcho}},
				},
			},
			expected: true,
		},
		{
			name: "app quit only",
			cfg: &config.Config{
				Hooks: config.HooksConfig{
					AppQuit: []config.HookEntry{{Run: hookRunEcho}},
				},
			},
			expected: true,
		},
		{
			name: "window events do not count as app events",
			cfg: &config.Config{
				Hooks: config.HooksConfig{
					WindowFocus: []config.HookEntry{{Run: hookRunEcho}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasAppEvents(tt.cfg)
			if result != tt.expected {
				t.Errorf("hasAppEvents() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestHasWorkspaceEvents(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected bool
	}{
		{
			name:     testCaseNameEmptyConfig,
			cfg:      &config.Config{},
			expected: false,
		},
		{
			name: "workspace changed only",
			cfg: &config.Config{
				Hooks: config.HooksConfig{
					WorkspaceChanged: []config.HookEntry{{Run: hookRunEcho}},
				},
			},
			expected: true,
		},
		{
			name: "window events do not count as workspace events",
			cfg: &config.Config{
				Hooks: config.HooksConfig{
					WindowFocus: []config.HookEntry{{Run: hookRunEcho}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasWorkspaceEvents(tt.cfg)
			if result != tt.expected {
				t.Errorf("hasWorkspaceEvents() = %v, expected %v", result, tt.expected)
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

	appOnlyObs := getObserverConfig(&config.Config{
		Hooks: config.HooksConfig{
			AppActivate: []config.HookEntry{{Run: hookRunEcho}},
		},
	})
	if !appOnlyObs.AppLifecycle {
		t.Error("expected AppLifecycle enabled for app-only config")
	}

	if appOnlyObs.Workspace {
		t.Error("expected Workspace disabled for app-only config")
	}
}
