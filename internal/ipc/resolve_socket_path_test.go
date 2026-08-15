package ipc_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/ipc"
)

// TestResolveSocketPath pins mimi#90: PR #88 widened config-path resolution
// so every `mimi action …` invocation now passes a real (never empty) config
// path through to ResolveSocketPath, which loads that config and returns its
// socket_file — including a custom one, where before #88 the CLI always
// looked on the default socket instead. This test pins the three cases the
// function itself promises, independent of how the CLI resolves the config
// path that reaches it.
func TestResolveSocketPath(t *testing.T) {
	t.Parallel()

	t.Run("a config path given returns the configured socket_file", func(t *testing.T) {
		t.Parallel()

		configPath := filepath.Join(t.TempDir(), "config.toml")
		wantSocket := filepath.Join(t.TempDir(), "custom.sock")

		writeTOML(t, configPath, "[settings]\nsocket_file = "+quoteTOML(wantSocket)+"\n")

		got := ipc.ResolveSocketPath(configPath)
		if got != wantSocket {
			t.Fatalf(
				"ResolveSocketPath(%q) = %q, want configured socket %q",
				configPath,
				got,
				wantSocket,
			)
		}
	})

	// The remaining cases all promise the same outcome — the default
	// socket — for different reasons ResolveSocketPath never reaches a
	// configured socket_file.
	tests := []struct {
		name       string
		configPath func(t *testing.T) string
	}{
		{
			name: "empty config path returns the default socket without touching disk",
			configPath: func(*testing.T) string {
				return ""
			},
		},
		{
			name: "a config path that fails to load returns the default socket",
			configPath: func(t *testing.T) string {
				t.Helper()

				// A path with no file behind it: config.Load fails to read
				// it, so ResolveSocketPath must fall back to the default
				// socket rather than propagate the load error or return an
				// empty path.
				return filepath.Join(t.TempDir(), "does-not-exist.toml")
			},
		},
		{
			name: "a config path with unparsable content returns the default socket",
			configPath: func(t *testing.T) string {
				t.Helper()

				configPath := filepath.Join(t.TempDir(), "broken.toml")
				writeTOML(t, configPath, "this is not valid = = toml\n")

				return configPath
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ipc.ResolveSocketPath(tt.configPath(t))
			if got != config.DefaultSocketPath {
				t.Fatalf(
					"ResolveSocketPath(...) = %q, want default %q",
					got,
					config.DefaultSocketPath,
				)
			}
		})
	}
}

// writeTOML writes body to path, failing the test on any I/O error.
func writeTOML(t *testing.T, path, body string) {
	t.Helper()

	err := os.WriteFile(path, []byte(body), 0o600)
	if err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// quoteTOML wraps s in double quotes for use as a TOML string value. Test
// inputs here never contain a quote or backslash, so no escaping is needed.
func quoteTOML(s string) string {
	return `"` + s + `"`
}
