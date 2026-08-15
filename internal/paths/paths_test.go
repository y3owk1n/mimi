package paths_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/y3owk1n/mimi/internal/paths"
)

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := paths.ExpandHome("~/.config/mimi/config.toml")
	want := filepath.Join(home, ".config", "mimi", "config.toml")

	if got != want {
		t.Errorf("ExpandHome() = %q, want %q", got, want)
	}
}

func TestExpandHome_NoTilde(t *testing.T) {
	got := paths.ExpandHome("/etc/mimi/config.toml")
	want := "/etc/mimi/config.toml"

	if got != want {
		t.Errorf("ExpandHome() = %q, want %q", got, want)
	}
}

// TestExpandHome_HomeUnresolvable exercises the failure branch of
// os.UserHomeDir(), which reads $HOME on Unix. Clearing it forces the
// lookup to fail. Per the ticket's ruling, ExpandHome must return the
// path unchanged (still containing the literal "~") rather than joining
// onto an empty string, which would silently produce a path relative to
// the current working directory.
func TestExpandHome_HomeUnresolvable(t *testing.T) {
	t.Setenv("HOME", "")

	const input = "~/.config/mimi/config.toml"

	got := paths.ExpandHome(input)

	if got != input {
		t.Errorf("ExpandHome() = %q, want unchanged %q", got, input)
	}

	if !strings.Contains(got, "~") {
		t.Errorf("ExpandHome() = %q, want it to retain the literal ~", got)
	}

	if filepath.IsAbs(got) {
		t.Errorf("ExpandHome() = %q, want it not to become an absolute/CWD-relative path", got)
	}
}
