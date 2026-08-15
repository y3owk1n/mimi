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

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
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

// runWithStubbedBody runs the command at path against a fresh tree whose body
// has been replaced by stub, and reports everything the tree printed and how
// the run ended. Stubbing the body is what lets a test drive the real tree
// without the command starting a daemon, moving a window or calling launchctl;
// the stub is handed the tree's root so a test can read what the tree resolved
// before the body ran.
func runWithStubbedBody(
	t *testing.T,
	path []string,
	stub func(root *cobra.Command) error,
) (string, error) {
	t.Helper()

	var out bytes.Buffer

	root := newRootCmd()

	target, _, err := root.Find(path)
	if err != nil {
		t.Fatalf("finding command: %v", err)
	}

	target.Args = cobra.ArbitraryArgs
	target.RunE = func(_ *cobra.Command, _ []string) error {
		return stub(root)
	}

	root.SetArgs(path)
	root.SetOut(&out)
	root.SetErr(&out)

	err = root.Execute()

	return out.String(), err
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

// TestNewRootCmd_ARuntimeFailurePrintsNoUsage is mimi#175's first half: a
// command that fails while it runs reports the failure and nothing else. Usage
// answers a mistyped command line; it says nothing about a daemon that is not
// running, and ten lines of flags after the sentence explaining the failure
// bury the sentence.
func TestNewRootCmd_ARuntimeFailurePrintsNoUsage(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfig(t, filepath.Join(xdg, "mimi", "config.toml"), "/marker/default.log")

	// No daemon runs under this test's temporary HOME, so reload fails on the
	// missing PID file — a real runtime failure rather than a bad command line.
	out, err := runCommand(t, "config", "reload")
	if err == nil {
		t.Fatal("expected config reload to fail without a running daemon")
	}

	if !strings.Contains(out, err.Error()) {
		t.Errorf("config reload never printed its failure, got: %s", out)
	}

	if strings.Contains(out, "Usage:") {
		t.Errorf("config reload printed usage after a runtime failure, got: %s", out)
	}
}

// TestNewRootCmd_AnArgumentFailureStillPrintsUsage is mimi#175's other half:
// the command lines cobra never understood keep their usage block, because
// there the list of flags and arguments is the answer to what went wrong.
func TestNewRootCmd_AnArgumentFailureStillPrintsUsage(t *testing.T) {
	testCases := []struct {
		name string
		argv []string
	}{
		{name: "an unknown flag", argv: []string{
			actionCommandName, focusWindowCommandName, "--not-a-flag",
		}},
		{name: "an unparsable flag value", argv: []string{
			actionCommandName, resizeWindowCommandName, "--width", "abc",
		}},
		{name: "a missing argument", argv: []string{
			actionCommandName, string(action.NameSpace),
		}},
		{name: "an extra argument", argv: []string{
			actionCommandName, string(action.NameSpace), "1", "extra",
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			isolateConfigHome(t)

			out, err := runCommand(t, testCase.argv...)
			if err == nil {
				t.Fatalf("%v: expected an error", testCase.argv)
			}

			if !strings.Contains(out, "Usage:") {
				t.Errorf("%v printed no usage, got: %s", testCase.argv, out)
			}
		})
	}
}

// TestActionCommand_WithoutASubcommandStillListsItsSubcommands is the case
// mimi#175's two halves meet in. `mimi action` fails from its run function, so
// cobra calls it a runtime failure, but what the user got wrong is the missing
// subcommand — and the usage block is the only place the CLI names the four
// subcommands that would fix it.
func TestActionCommand_WithoutASubcommandStillListsItsSubcommands(t *testing.T) {
	isolateConfigHome(t)

	out, err := runCommand(t, actionCommandName)
	if err == nil {
		t.Fatal("expected mimi action to fail without a subcommand")
	}

	if !strings.Contains(out, "Usage:") {
		t.Errorf("mimi action printed no usage, got: %s", out)
	}

	for _, name := range []string{
		focusWindowCommandName,
		resizeWindowCommandName,
		string(action.NameSpace),
		string(action.NameMoveWindowToSpace),
	} {
		if !strings.Contains(out, name) {
			t.Errorf("mimi action never named the %q subcommand, got: %s", name, out)
		}
	}
}

// TestNewRootCmd_NoCommandPrintsUsageAfterARuntimeFailure walks the whole tree
// because mimi#175 changes what every failing command prints, not just the one
// a test happened to pick. Each command's own work is stubbed out with a
// failure, so what is under test is the tree's reaction to a body that returned
// an error rather than the body itself — including for "action", whose real
// body is a usage failure the test above covers instead.
func TestNewRootCmd_NoCommandPrintsUsageAfterARuntimeFailure(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfig(t, filepath.Join(xdg, "mimi", "config.toml"), "/marker/default.log")

	wantErr := derrors.New(derrors.CodeInternal, "the command failed while running")

	paths := runnableCommandPaths(newRootCmd())
	if len(paths) == 0 {
		t.Fatal("found no runnable commands in the tree")
	}

	for _, path := range paths {
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			out, err := runWithStubbedBody(t, path, func(_ *cobra.Command) error {
				return wantErr
			})
			if err == nil {
				t.Fatalf("%v: expected the stubbed failure", path)
			}

			if !strings.Contains(out, wantErr.Error()) {
				t.Errorf("%v never printed its failure, got: %s", path, out)
			}

			if strings.Contains(out, "Usage:") {
				t.Errorf("%v printed usage after a runtime failure, got: %s", path, out)
			}
		})
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
			var seen string

			_, err := runWithStubbedBody(t, path, func(root *cobra.Command) error {
				seen = configFlagValue(root)

				return nil
			})
			if err != nil {
				t.Fatalf("running command: %v", err)
			}

			if seen != want {
				t.Errorf("command saw config path %q, want %q", seen, want)
			}
		})
	}
}
