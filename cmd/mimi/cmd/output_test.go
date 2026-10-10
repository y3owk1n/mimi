//nolint:testpackage // drives the command tree as main does
package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cliArgsEnv makes the copy of this test binary that runCLI starts run a
// fresh command tree with these arguments, its stdout and stderr left as
// main has them.
const cliArgsEnv = "MIMI_TEST_CLI_ARGS"

func TestHelperCLI(t *testing.T) {
	args := os.Getenv(cliArgsEnv)
	if args == "" {
		t.Skip("only runs as the CLI runCLI starts")
	}

	root := newRootCmd()
	root.SetArgs(strings.Split(args, "\n"))

	err := root.Execute()
	if err != nil {
		os.Exit(1)
	}
}

// runCLI runs args in a copy of this test binary, with nothing redirected,
// and returns what it wrote to stdout and to stderr, and how it exited.
func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestHelperCLI$")

	cmd.Env = append(os.Environ(), cliArgsEnv+"="+strings.Join(args, "\n"))

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}

func TestCLI_PrintsItsOutputOnStdout(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfigFile(t, path, "")

	stdout, stderr, err := runCLI(t, "config", "dump", "--config", path)
	if err != nil {
		t.Fatalf("config dump: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, `"socketFile"`) {
		t.Errorf("config dump should print the config on stdout, got stdout:\n%s", stdout)
	}

	if stderr != "" {
		t.Errorf("config dump should print nothing on stderr, got:\n%s", stderr)
	}
}

func TestCLI_PrintsUsageForABadFlagOnStderr(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runCLI(t, "status", "--nope")
	if err == nil {
		t.Fatal("an unknown flag should fail the command")
	}

	if stdout != "" {
		t.Errorf("a bad flag should print nothing on stdout, got:\n%s", stdout)
	}

	if !strings.Contains(stderr, "unknown flag: --nope") || !strings.Contains(stderr, "Usage:") {
		t.Errorf("a bad flag should print its error and usage on stderr, got:\n%s", stderr)
	}
}
