//nolint:testpackage
package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctor_ReportsABrokenConfigAndExitsNonZero(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfigFile(t, filepath.Join(xdg, "mimi", "config.toml"), "[hooks\n")

	out, err := runCommand(t, "doctor")
	if err == nil {
		t.Fatal("doctor exited 0 over a config that does not parse")
	}

	if !strings.Contains(out, "FAIL  config") {
		t.Fatalf("no failed config line in:\n%s", out)
	}
}

func TestDoctor_WarnsAboutAHookCommandOffTheServicePath(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfigFile(t, filepath.Join(xdg, "mimi", "config.toml"), strings.Join([]string{
		"[settings]",
		`service_path = "/nonexistent"`,
		"[hooks]",
		`on_app_activate = [{ run = "definitely-not-installed --flag" }]`,
		"",
	}, "\n"))

	out, _ := runCommand(t, "doctor")

	if !strings.Contains(
		out,
		"warn  hook commands  not on the service PATH: definitely-not-installed",
	) {
		t.Fatalf("no hook command warning in:\n%s", out)
	}
}
