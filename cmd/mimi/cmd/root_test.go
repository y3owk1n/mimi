//nolint:testpackage
package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const (
	testDirPerm  = 0o755
	testFilePerm = 0o600
)

// isolateConfigHome points HOME, XDG_CONFIG_HOME and the working directory at
// throwaway directories, so no test in this file can read or write the config
// of whoever is running it. It returns the XDG config directory.
func isolateConfigHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	xdg := filepath.Join(home, ".config")

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Chdir(t.TempDir())

	return xdg
}

// writeConfig writes a minimal valid config whose log_file marks which file a
// command actually loaded.
func writeConfig(t *testing.T, path, logFile string) {
	t.Helper()

	writeConfigFile(t, path, fmt.Sprintf("[settings]\nlog_file = %q\n", logFile))
}

// writeConfigFile writes body to path, creating the directories above it.
func writeConfigFile(t *testing.T, path, body string) {
	t.Helper()

	err := os.MkdirAll(filepath.Dir(path), testDirPerm)
	if err != nil {
		t.Fatalf("creating config directory: %v", err)
	}

	err = os.WriteFile(path, []byte(body), testFilePerm)
	if err != nil {
		t.Fatalf("writing config: %v", err)
	}
}

// runCommand runs args against a fresh command tree and returns everything the
// tree printed.
func runCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer

	root := newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)

	err := root.Execute()

	return out.String(), err
}

// configFlagValue reports the config path the tree holds, which the persistent
// --config flag and the resolved path share.
func configFlagValue(root *cobra.Command) string {
	return root.PersistentFlags().Lookup("config").Value.String()
}

// runnableCommandPaths lists the argument path of every command in the tree
// that has something to run.
func runnableCommandPaths(root *cobra.Command) [][]string {
	var paths [][]string

	var walk func(parent *cobra.Command, prefix []string)

	walk = func(parent *cobra.Command, prefix []string) {
		for _, child := range parent.Commands() {
			// Cobra's own generated commands are not mimi's to test.
			if child.Name() == "help" || child.Name() == "completion" {
				continue
			}

			path := append(append([]string{}, prefix...), child.Name())
			if child.Runnable() {
				paths = append(paths, path)
			}

			walk(child, path)
		}
	}

	walk(root, nil)

	return paths
}

func TestConfigDump_WithoutConfigFlag_LoadsTheDefaultConfigPath(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfig(t, filepath.Join(xdg, "mimi", "config.toml"), "/marker/default.log")

	out, err := runCommand(t, "config", "dump")
	if err != nil {
		t.Fatalf("config dump: %v", err)
	}

	if !strings.Contains(out, "/marker/default.log") {
		t.Errorf("config dump did not load the default config, got: %s", out)
	}
}

func TestConfigValidate_WithoutConfigFlag_ValidatesTheDefaultConfigPath(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfig(t, filepath.Join(xdg, "mimi", "config.toml"), "/marker/default.log")

	out, err := runCommand(t, "config", "validate")
	if err != nil {
		t.Fatalf("config validate: %v", err)
	}

	if !strings.Contains(out, "Config valid") {
		t.Errorf("config validate did not accept the default config, got: %s", out)
	}
}

func TestConfigInit_WithoutConfigFlag_WritesToTheDefaultConfigPath(t *testing.T) {
	xdg := isolateConfigHome(t)
	want := filepath.Join(xdg, "mimi", "config.toml")

	out, err := runCommand(t, "config", "init")
	if err != nil {
		t.Fatalf("config init: %v", err)
	}

	_, err = os.Stat(want)
	if err != nil {
		t.Fatalf("config init wrote no config at %s: %v (output: %s)", want, err, out)
	}
}

func TestConfigReload_WithoutConfigFlag_GetsPastTheConfigPath(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfig(t, filepath.Join(xdg, "mimi", "config.toml"), "/marker/default.log")

	// The daemon is not running under this test's temporary HOME, so reload is
	// expected to fail — but on the missing PID file, never on the config path.
	_, err := runCommand(t, "config", "reload")
	if err == nil {
		t.Fatal("expected config reload to fail without a running daemon")
	}

	if !strings.Contains(err.Error(), "reading pid file") {
		t.Errorf("config reload failed before reaching the PID file: %v", err)
	}
}

func TestConfigDump_WithExplicitConfigFlag_PrefersTheGivenFile(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfig(t, filepath.Join(xdg, "mimi", "config.toml"), "/marker/default.log")

	explicit := filepath.Join(t.TempDir(), "explicit.toml")
	writeConfig(t, explicit, "/marker/explicit.log")

	out, err := runCommand(t, "config", "dump", "-c", explicit)
	if err != nil {
		t.Fatalf("config dump -c: %v", err)
	}

	if !strings.Contains(out, "/marker/explicit.log") {
		t.Errorf("config dump ignored the explicit -c file, got: %s", out)
	}

	if strings.Contains(out, "/marker/default.log") {
		t.Errorf("config dump loaded the default config despite -c, got: %s", out)
	}
}

func TestNewRootCmd_KeepsNoStateBetweenTrees(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfig(t, filepath.Join(xdg, "mimi", "config.toml"), "/marker/default.log")

	explicit := filepath.Join(t.TempDir(), "explicit.toml")
	writeConfig(t, explicit, "/marker/explicit.log")

	mutated := newRootCmd()
	mutated.SetArgs([]string{"config", "dump", "-c", explicit})
	mutated.SetOut(io.Discard)
	mutated.SetErr(io.Discard)

	err := mutated.Execute()
	if err != nil {
		t.Fatalf("config dump -c: %v", err)
	}

	fresh := newRootCmd()
	if got := configFlagValue(fresh); got != "" {
		t.Fatalf("a fresh tree started with config %q, want an empty flag", got)
	}

	out, err := runCommand(t, "config", "dump")
	if err != nil {
		t.Fatalf("config dump: %v", err)
	}

	if !strings.Contains(out, "/marker/default.log") {
		t.Errorf("the previous tree's -c leaked into the next tree, got: %s", out)
	}
}

// TestNewRootCmd_EveryCommandResolvesTheConfigPath is the guard the config
// family lacked: it walks the whole tree rather than one family, because a
// command whose config path is never resolved fails the same way anywhere.
//
// Each command's own work is stubbed out — running it for real would start a
// daemon, move windows or call launchctl — so what is under test is that the
// tree hands the command a resolved config path before it runs.
func TestNewRootCmd_EveryCommandResolvesTheConfigPath(t *testing.T) {
	xdg := isolateConfigHome(t)
	want := filepath.Join(xdg, "mimi", "config.toml")
	writeConfig(t, want, "/marker/default.log")

	paths := runnableCommandPaths(newRootCmd())
	if len(paths) == 0 {
		t.Fatal("found no runnable commands in the tree")
	}

	for _, path := range paths {
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			root := newRootCmd()

			target, _, err := root.Find(path)
			if err != nil {
				t.Fatalf("finding command: %v", err)
			}

			var seen string

			target.Args = cobra.ArbitraryArgs
			target.RunE = func(_ *cobra.Command, _ []string) error {
				seen = configFlagValue(root)

				return nil
			}

			root.SetArgs(path)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)

			err = root.Execute()
			if err != nil {
				t.Fatalf("running command: %v", err)
			}

			if seen != want {
				t.Errorf("command saw config path %q, want %q", seen, want)
			}
		})
	}
}
