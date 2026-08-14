//nolint:testpackage
package cmd

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/y3owk1n/mimi/internal/config"
)

// writeEveryHookKind writes a config carrying one hook of every kind the config
// supports and returns how many kinds that is.
//
// The kinds come from config.HooksConfig itself rather than a list kept here,
// so a hook kind added to the config is covered without anyone remembering to
// update this test.
func writeEveryHookKind(t *testing.T, path string) int {
	t.Helper()

	hooks := reflect.TypeFor[config.HooksConfig]()
	entries := reflect.TypeFor[[]config.HookEntry]()
	lines := []string{"[hooks]"}

	for field := range hooks.Fields() {
		if field.Type != entries {
			continue
		}

		name, _, _ := strings.Cut(field.Tag.Get("toml"), ",")
		if name == "" {
			t.Fatalf("hook field %s carries no toml tag", field.Name)
		}

		lines = append(lines, fmt.Sprintf("%s = [%q]", name, "true"))
	}

	kinds := len(lines) - 1
	if kinds == 0 {
		t.Fatal("found no hook kinds on config.HooksConfig")
	}

	writeConfigFile(t, path, strings.Join(lines, "\n")+"\n")

	return kinds
}

func TestConfigValidate_CountsAHookOfEveryKind(t *testing.T) {
	xdg := isolateConfigHome(t)
	want := writeEveryHookKind(t, filepath.Join(xdg, "mimi", "config.toml"))

	out, err := runCommand(t, "config", "validate")
	if err != nil {
		t.Fatalf("config validate: %v", err)
	}

	report := fmt.Sprintf("Config valid (%d hook(s) defined)", want)
	if !strings.Contains(out, report) {
		t.Errorf(
			"config validate printed %q, want it to contain %q",
			strings.TrimSpace(out),
			report,
		)
	}
}
