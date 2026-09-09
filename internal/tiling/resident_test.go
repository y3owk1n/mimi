package tiling_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/tiling"
)

// countingLayout answers every line with the number of lines it has read,
// so a test can tell one process from a restarted one.
const countingLayout = `n=0; while IFS= read -r line; do n=$((n+1)); echo "{\"frames\":[],\"state\":$n}"; done`

func stateOf(t *testing.T, out tiling.Output) int {
	t.Helper()

	var state int

	err := json.Unmarshal(out.State, &state)
	if err != nil {
		t.Fatalf("state %q is not a number: %v", out.State, err)
	}

	return state
}

func TestResident_KeepsOneProcessAcrossPasses(t *testing.T) {
	t.Parallel()

	layout := tiling.NewResident(shell, countingLayout, time.Second, nil)
	defer layout.Stop()

	for want := 1; want <= 3; want++ {
		out, err := layout.Reduce(context.Background(), tiling.Input{})
		if err != nil {
			t.Fatalf("Reduce() #%d error = %v, want nil", want, err)
		}

		if got := stateOf(t, out); got != want {
			t.Fatalf("Reduce() #%d state = %d, want %d (a new process per pass)", want, got, want)
		}
	}

	layout.Stop()

	out, err := layout.Reduce(context.Background(), tiling.Input{})
	if err != nil {
		t.Fatalf("Reduce() after Stop error = %v, want nil", err)
	}

	if got := stateOf(t, out); got != 1 {
		t.Fatalf("Reduce() after Stop state = %d, want 1 (a fresh process)", got)
	}
}

func TestResident_RestartsALayoutThatExitedBetweenPasses(t *testing.T) {
	t.Parallel()

	// Answers once, then exits.
	layout := tiling.NewResident(
		shell,
		`IFS= read -r line; echo '{"frames":[],"state":1}'`,
		time.Second,
		nil,
	)
	defer layout.Stop()

	for pass := 1; pass <= 3; pass++ {
		out, err := layout.Reduce(context.Background(), tiling.Input{})
		if err != nil {
			t.Fatalf("Reduce() #%d error = %v, want nil", pass, err)
		}

		if got := stateOf(t, out); got != 1 {
			t.Fatalf("Reduce() #%d state = %d, want 1", pass, got)
		}
	}
}

func TestResident_FailuresAreReportedAndBackedOff(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		command  string
		fragment string
	}{
		"exits without answering": {`echo nope >&2; exit 3`, "nope"},
		"prints something else": {
			`while IFS= read -r line; do echo not json; done`,
			"decoding layout output",
		},
		"never answers": {`while IFS= read -r line; do sleep 5; done`, "timed out"},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			layout := tiling.NewResident(shell, testCase.command, 200*time.Millisecond, nil)
			defer layout.Stop()

			_, err := layout.Reduce(context.Background(), tiling.Input{})
			if !derrors.IsCode(err, derrors.CodeActionFailed) &&
				!derrors.IsCode(err, derrors.CodeSerializationFailed) {
				t.Fatalf("Reduce() error = %v, want a failure", err)
			}

			if !strings.Contains(err.Error(), testCase.fragment) {
				t.Fatalf("error %q does not mention %q", err.Error(), testCase.fragment)
			}

			// Right after a failure the layout is not started again.
			_, err = layout.Reduce(context.Background(), tiling.Input{})
			if err == nil || !strings.Contains(err.Error(), "restarting in") {
				t.Fatalf("Reduce() right after a failure error = %v, want a backoff", err)
			}
		})
	}
}
