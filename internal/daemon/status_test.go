//nolint:testpackage // tests statusAnswer, an unexported function
package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/systray"
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
		true,
		func() *config.Config { return cfg },
		func() *Reload { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	var status Status

	err = json.Unmarshal(data, &status)
	if err != nil {
		t.Fatal(err)
	}

	if status.Version != "v9.9.9" || status.ConfigPath != "/x/config.toml" {
		t.Fatalf("got %+v", status)
	}

	if status.Accessibility == nil || !*status.Accessibility {
		t.Fatalf("accessibility %v, want the grant the daemon holds now", status.Accessibility)
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

// TestStatusAnswer_ReportsHowTheLastReloadWent pins what mimi config reload
// reads back after it signals: whether the file applied, why not when it did
// not, and the settings it changed that need a restart.
func TestStatusAnswer_ReportsHowTheLastReloadWent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfig := func(content string) {
		t.Helper()

		err := os.WriteFile(path, []byte(content), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	writeConfig("")

	running, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	cfgReloader, _ := newTestReloader(t, running)
	reload := func() Reload {
		t.Helper()

		reloadConfig(path, cfgReloader, reloadTriggerSighup, func(systray.ReloadOutcome) {},
			zap.NewNop().Sugar())

		data, err := statusAnswer("v", path, time.Now(), true, true, cfgReloader.Current,
			cfgReloader.LastReload)
		if err != nil {
			t.Fatal(err)
		}

		var status Status

		err = json.Unmarshal(data, &status)
		if err != nil {
			t.Fatal(err)
		}

		if status.LastReload == nil {
			t.Fatal("no last reload reported after a reload")
		}

		return *status.LastReload
	}

	writeConfig("[hooks\n")

	failed := reload()
	if failed.OK || failed.Error == "" || failed.Trigger != string(reloadTriggerSighup) {
		t.Fatalf("a config that does not load reported %+v, want a failure with its reason", failed)
	}

	writeConfig("[settings]\nlog_level = \"debug\"\n")

	applied := reload()
	if !applied.OK || applied.Error != "" || !applied.At.After(failed.At) {
		t.Fatalf("a config that loads reported %+v, want a later success", applied)
	}

	if !slices.ContainsFunc(applied.RestartOnly, func(key string) bool {
		return strings.HasSuffix(key, "log_level")
	}) {
		t.Fatalf("restart-only settings %v, want log_level among them", applied.RestartOnly)
	}
}
