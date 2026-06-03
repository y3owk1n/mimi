//nolint:testpackage
package daemon

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/config"
)

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
					WindowFocus: []config.HookEntry{{Run: "echo"}},
				},
			},
			expected: true,
		},
		{
			name: "window created only",
			cfg: &config.Config{
				Hooks: config.HooksConfig{
					WindowCreated: []config.HookEntry{{Run: "echo"}},
				},
			},
			expected: true,
		},
		{
			name: "app activate only",
			cfg: &config.Config{
				Hooks: config.HooksConfig{
					AppActivate: []config.HookEntry{{Run: "echo"}},
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
			AppActivate:              []config.HookEntry{{Run: "echo"}},
			AudioDeviceChanged:       []config.HookEntry{{Run: "echo"}},
			BatteryLow:               []config.HookEntry{{Run: "echo"}},
			ClipboardChanged:         []config.HookEntry{{Run: "echo"}},
			USBDeviceConnected:       []config.HookEntry{{Run: "echo"}},
			NetworkUp:                []config.HookEntry{{Run: "echo"}},
			VolumeMount:              []config.HookEntry{{Run: "echo"}},
			WorkspaceChanged:         []config.HookEntry{{Run: "echo"}},
			AppearanceChanged:        []config.HookEntry{{Run: "echo"}},
			ExternalDisplayConnected: []config.HookEntry{{Run: "echo"}},
			SystemSleep:              []config.HookEntry{{Run: "echo"}},
		},
	}

	obs := getObserverConfig(cfg)

	if !obs.AppLifecycle {
		t.Error("expected AppLifecycle to be true")
	}

	if !obs.Audio {
		t.Error("expected Audio to be true")
	}

	if !obs.Power {
		t.Error("expected Power to be true")
	}

	if !obs.Clipboard {
		t.Error("expected Clipboard to be true")
	}

	if !obs.USB {
		t.Error("expected USB to be true")
	}

	if !obs.Network {
		t.Error("expected Network to be true")
	}

	if !obs.Volume {
		t.Error("expected Volume to be true")
	}

	if !obs.Workspace {
		t.Error("expected Workspace to be true")
	}

	if !obs.Appearance {
		t.Error("expected Appearance to be true")
	}

	if !obs.Display {
		t.Error("expected Display to be true")
	}

	if !obs.SystemState {
		t.Error("expected SystemState to be true")
	}

	// Empty config case
	emptyCfg := &config.Config{}

	emptyObs := getObserverConfig(emptyCfg)
	if emptyObs.AppLifecycle ||
		emptyObs.Audio ||
		emptyObs.Power ||
		emptyObs.Clipboard ||
		emptyObs.USB ||
		emptyObs.Network ||
		emptyObs.Volume ||
		emptyObs.Workspace ||
		emptyObs.Appearance ||
		emptyObs.Display ||
		emptyObs.SystemState {
		t.Errorf("expected all observers to be disabled on empty config, got: %+v", emptyObs)
	}
}
