//nolint:testpackage
package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

const hooksConfig = `[hooks]
on_app_activate = [
    { run = "echo active: $mimi_APP_NAME", app = "Safari" },
    { run = "exit 3", app = "!Safari", async = true, timeout_secs = 2 },
]
on_workspace_changed = ["echo space $mimi_SPACE_INDEX"]
`

func TestHooksList_PrintsEveryEntryWithItsSettings(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfigFile(t, filepath.Join(xdg, "mimi", "config.toml"), hooksConfig)

	out, err := runCommand(t, "hooks", "list")
	if err != nil {
		t.Fatalf("hooks list: %v", err)
	}

	want := "on_app_activate\n" +
		"  [0] echo active: $mimi_APP_NAME\n" +
		"      app=Safari\n" +
		"  [1] exit 3\n" +
		"      app=!Safari timeout_secs=2 async\n" +
		"on_workspace_changed\n" +
		"  [0] echo space $mimi_SPACE_INDEX\n"
	if out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestHooksFire_ReportsEachHookMatchedOrSkipped(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfigFile(t, filepath.Join(xdg, "mimi", "config.toml"), hooksConfig)

	out, err := runCommand(t, "hooks", "fire", "on_app_activate", "--app", "Safari")
	if err != nil {
		t.Fatalf("hooks fire: %v\n%s", err, out)
	}

	if !strings.Contains(out, "[0] matched, ok in") ||
		!strings.Contains(out, "    active: Safari\n") {
		t.Fatalf("the matched hook was not reported with its output:\n%s", out)
	}

	if !strings.Contains(out, "[1] skipped: app filter mismatch\n") {
		t.Fatalf("the skipped hook was not reported:\n%s", out)
	}
}

func TestHooksFire_ExitsNonZeroWhenAHookFails(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfigFile(t, filepath.Join(xdg, "mimi", "config.toml"), hooksConfig)

	out, err := runCommand(t, "hooks", "fire", "app_activate", "--app", "Mail")
	if err == nil {
		t.Fatalf("a failing hook exited 0:\n%s", out)
	}

	if !strings.Contains(out, "[1] matched, failed in") {
		t.Fatalf("the failure was not reported:\n%s", out)
	}
}

func TestHooksFire_ExtraReachesTheHook(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfigFile(t, filepath.Join(xdg, "mimi", "config.toml"), hooksConfig)

	out, err := runCommand(t, "hooks", "fire", "on_workspace_changed", "--extra", "space_index=4")
	if err != nil {
		t.Fatalf("hooks fire: %v\n%s", err, out)
	}

	if !strings.Contains(out, "    space 4\n") {
		t.Fatalf("the extra did not reach the hook:\n%s", out)
	}
}

func TestHooksFire_RejectsAnUnknownKind(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfigFile(t, filepath.Join(xdg, "mimi", "config.toml"), hooksConfig)

	_, err := runCommand(t, "hooks", "fire", "on_coffee")
	if err == nil || !strings.Contains(err.Error(), "unknown hook kind") {
		t.Fatalf("got %v", err)
	}
}

func TestHooksTail_SaysSoWithoutADaemon(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfigFile(t, filepath.Join(xdg, "mimi", "config.toml"), hooksConfig)

	_, err := runCommand(t, "hooks", "tail")
	if err == nil || !strings.Contains(err.Error(), "no daemon is running") {
		t.Fatalf("got %v", err)
	}
}
