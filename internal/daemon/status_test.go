//nolint:testpackage // tests statusAnswer, an unexported function
package daemon

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/ipc"
)

func TestStatusAnswer_ReportsTheBuildAndTheConfigInEffect(t *testing.T) {
	cfg := &config.Config{}
	cfg.Hooks.AppActivate = []config.HookEntry{{Run: "true"}, {Run: "true"}}
	cfg.Tiling.Enabled = true
	cfg.Border.Enabled = true

	data, err := statusAnswer(
		"v9.9.9",
		"/x/config.toml",
		time.Now().Add(-90*time.Second),
		false,
		func() *config.Config { return cfg },
	)
	if err != nil {
		t.Fatal(err)
	}

	var status Status

	err = json.Unmarshal(data, &status)
	if err != nil {
		t.Fatal(err)
	}

	if status.Version != "v9.9.9" || status.Protocol != ipc.ProtocolVersion ||
		status.ConfigPath != "/x/config.toml" {
		t.Fatalf("got %+v", status)
	}

	if status.UptimeSecs < 90 {
		t.Fatalf("uptime %d, want at least 90", status.UptimeSecs)
	}

	if status.Features.Hooks != 2 {
		t.Fatalf("hooks %d, want 2", status.Features.Hooks)
	}

	// Tiling and borders are enabled in the config but not in effect without
	// Accessibility, and the answer reports what is in effect.
	if status.Features.Tiling || status.Features.Borders {
		t.Fatalf("window features reported on without Accessibility: %+v", status.Features)
	}
}
