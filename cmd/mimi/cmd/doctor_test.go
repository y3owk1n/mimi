//nolint:testpackage
package cmd

import (
	"encoding/json"
	"path/filepath"
	"regexp"
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

	warning := regexp.MustCompile(
		`warn\s+hook commands\s+not on the service PATH: definitely-not-installed`,
	)
	if !warning.MatchString(out) {
		t.Fatalf("no hook command warning in:\n%s", out)
	}
}

// TestDoctor_JSONPrintsEveryCheckAndStillExitsNonZero pins the --json form a
// script reads: the first line is the checks as JSON, each with its name and
// one of the four statuses, and a failed check still fails the command.
func TestDoctor_JSONPrintsEveryCheckAndStillExitsNonZero(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfigFile(t, filepath.Join(xdg, "mimi", "config.toml"), "[settings\n")

	out, err := runCommand(t, "doctor", "--json")
	if err == nil {
		t.Fatal("doctor --json exited 0 with a broken config")
	}

	firstLine, _, _ := strings.Cut(out, "\n")

	var checks []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}

	jsonErr := json.Unmarshal([]byte(firstLine), &checks)
	if jsonErr != nil {
		t.Fatalf("first line is not the checks as JSON: %v\n%s", jsonErr, out)
	}

	statuses := map[string]bool{"ok": true, "warn": true, "fail": true, "skip": true}
	failedConfig := false

	for _, check := range checks {
		if check.Name == "" || !statuses[check.Status] {
			t.Fatalf("check %+v, want a name and one of ok, warn, fail, skip", check)
		}

		failedConfig = failedConfig || (check.Name == "config" && check.Status == "fail")
	}

	if !failedConfig {
		t.Fatalf("checks %+v, want the config check failed", checks)
	}
}
