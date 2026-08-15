//nolint:testpackage // tests configProblems, an unexported function
package cmd

import (
	"strings"
	"testing"

	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// configProblems is where `mimi config validate` decides whether to fail, so
// this is what pins the exit-1 contract. The command itself is a thin shell
// that prints this text and calls os.Exit, which a test cannot survive.

func TestConfigProblems_SaysNothingAboutAGoodConfig(t *testing.T) {
	t.Parallel()

	if got := configProblems(&config.Config{}, nil); got != "" {
		t.Errorf("a good config should produce no report, got %q", got)
	}
}

func TestConfigProblems_NamesAnUnrecognizedKeyAndListsTheValidOnes(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{UnknownHookKeys: []string{"on_window_focussed"}}

	got := configProblems(cfg, nil)
	if got == "" {
		t.Fatal("an unrecognized hook key must be reported")
	}

	if !strings.Contains(got, "hooks.on_window_focussed: not a recognized hook kind") {
		t.Errorf("report should name the key the user typed, got:\n%s", got)
	}

	// The suggestion is the whole point: it turns "wrong" into "here is right".
	for _, name := range config.HookKindNames() {
		if !strings.Contains(got, name) {
			t.Errorf("report should list the recognized kind %q, got:\n%s", name, got)
		}
	}
}

// TestConfigProblems_ReportsAnUnrecognizedKeyAlongsideOtherErrors is the
// regression guard for the reason Load returns a config with its validation
// error. Reporting only the error hides the typo until the user fixes
// something unrelated and runs again -- which is the silent-drop defect this
// whole change exists to remove, wearing a different hat.
func TestConfigProblems_ReportsAnUnrecognizedKeyAlongsideOtherErrors(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{UnknownHookKeys: []string{"on_window_focussed"}}
	loadErr := derrors.New(
		derrors.CodeInvalidConfig,
		"hooks.on_app_activate[0]: run command is empty",
	)

	got := configProblems(cfg, loadErr)

	if !strings.Contains(got, "run command is empty") {
		t.Errorf("report should carry the load error, got:\n%s", got)
	}

	if !strings.Contains(got, "hooks.on_window_focussed: not a recognized hook kind") {
		t.Errorf("report should name the unrecognized key too, got:\n%s", got)
	}
}

func TestConfigProblems_ToleratesANilConfig(t *testing.T) {
	t.Parallel()

	// Load returns a nil config for failures earlier than validation.
	loadErr := derrors.New(derrors.CodeConfigIOFailed, "reading config")

	got := configProblems(nil, loadErr)
	if !strings.Contains(got, "reading config") {
		t.Errorf("report should carry the load error, got:\n%s", got)
	}
}
