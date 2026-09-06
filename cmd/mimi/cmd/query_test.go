//nolint:testpackage
package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

const (
	queryCommandName       = "query"
	querySpaceCommandName  = "space"
	queryWindowCommandName = "window"
)

// answerOn runs answerQuery against a command whose streams are captured, and
// returns what reached stdout and stderr.
func answerOn[T any](t *testing.T, query func() (T, error)) (string, string, error) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	cobraCmd := &cobra.Command{}
	cobraCmd.SetOut(&stdout)
	cobraCmd.SetErr(&stderr)

	err := answerQuery(cobraCmd, query)

	return stdout.String(), stderr.String(), err
}

// TestAnswerQuery_PrintsOneLineOfJSON pins the output format a script reads:
// the answer, as compact JSON, on one line, followed by a newline and nothing
// else.
func TestAnswerQuery_PrintsOneLineOfJSON(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		answer any
		want   string
	}{
		{
			name:   querySpaceCommandName,
			answer: action.SpaceInfo{Index: 2, Count: 5},
			want:   `{"index":2,"count":5}` + "\n",
		},
		{
			name: queryWindowCommandName,
			answer: action.WindowInfo{
				PID:   4242,
				Frame: action.Frame{X: 100, Y: 50, Width: 1024, Height: 768},
			},
			want: `{"pid":4242,"frame":{"x":100,"y":50,"width":1024,"height":768}}` + "\n",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := answerOn(t, func() (any, error) { return testCase.answer, nil })
			if err != nil {
				t.Fatalf("answerQuery() error = %v, want nil", err)
			}

			if stdout != testCase.want {
				t.Errorf("stdout = %q, want %q", stdout, testCase.want)
			}

			if stderr != "" {
				t.Errorf("stderr = %q, want nothing", stderr)
			}
		})
	}
}

// TestAnswerQuery_AFailurePrintsNothingOnStdout is the contract a script
// depends on: stdout carries an answer or nothing, never an error dressed as
// one.
func TestAnswerQuery_AFailurePrintsNothingOnStdout(t *testing.T) {
	t.Parallel()

	failure := derrors.New(derrors.CodeActionFailed, "no active window found")

	stdout, _, err := answerOn(t, func() (action.WindowInfo, error) {
		return action.WindowInfo{}, failure
	})
	if err == nil {
		t.Fatal("answerQuery() error = nil, want the query's failure")
	}

	if !derrors.IsCode(err, derrors.CodeActionFailed) {
		t.Errorf("error code = %q, want %q", derrors.GetCode(err), derrors.CodeActionFailed)
	}

	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
}

func TestQueryCommand_WithoutASubcommandStillListsItsSubcommands(t *testing.T) {
	isolateConfigHome(t)

	out, err := runCommand(t, queryCommandName)
	if err == nil {
		t.Fatal("expected mimi query to fail without a subcommand")
	}

	if !strings.Contains(out, "Usage:") {
		t.Errorf("mimi query printed no usage, got: %s", out)
	}

	for _, name := range []string{querySpaceCommandName, queryWindowCommandName} {
		if !strings.Contains(out, name) {
			t.Errorf("mimi query never named the %q subcommand, got: %s", name, out)
		}
	}
}

// TestQueryCommands_TakeNoArguments: a query has nothing to be told, so a
// stray argument is a mistake worth rejecting before any desktop read runs.
func TestQueryCommands_TakeNoArguments(t *testing.T) {
	isolateConfigHome(t)

	for _, name := range []string{querySpaceCommandName, queryWindowCommandName} {
		t.Run(name, func(t *testing.T) {
			_, err := runCommand(t, queryCommandName, name, "extra")
			if err == nil {
				t.Fatalf("mimi query %s extra reported success, want an argument error", name)
			}
		})
	}
}
