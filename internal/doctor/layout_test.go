package doctor_test

import (
	"strings"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/doctor"
)

const (
	shell  = "/bin/sh"
	sample = "bsp.py"
)

func TestRunLayout_CountsTheFramesALayoutAnswers(t *testing.T) {
	command := `printf '{"frames":[{"number":1,"frame":{"x":0,"y":0,"width":1,"height":1}},` +
		`{"number":2,"frame":{"x":1,"y":0,"width":1,"height":1}}],"state":null}'`

	run := doctor.RunLayout(t.Context(), shell, command, time.Second)
	if run.Err != nil || run.Frames != 2 || run.Windows != 2 || run.Empty {
		t.Fatalf("got %+v", run)
	}

	check := doctor.Assess(doctor.Facts{Config: &config.Config{}, Layouts: []doctor.LayoutRun{run}})
	if got := statusOf(t, check, "layout"); got.Status != doctor.Pass {
		t.Fatalf("got %+v", got)
	}
}

func TestRunLayout_ReportsEachWayALayoutBreaks(t *testing.T) {
	cases := []struct {
		name, command string
		want          doctor.Status
		detail        string
	}{
		{"exits non-zero", "echo boom >&2; exit 1", doctor.Fail, "boom"},
		{"prints bad JSON", "echo '{'", doctor.Fail, "decoding layout output"},
		{"times out", "sleep 5", doctor.Fail, "timed out"},
		{"prints nothing", "cat >/dev/null", doctor.Warn, "printed nothing"},
		{
			"moves nothing",
			`echo '{"frames":[],"state":null}'`,
			doctor.Warn,
			"no frames for 2 windows",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			run := doctor.RunLayout(t.Context(), shell, testCase.command, 200*time.Millisecond)

			facts := doctor.Facts{Config: &config.Config{}, Layouts: []doctor.LayoutRun{run}}

			check := statusOf(t, doctor.Assess(facts), "layout")
			if check.Status != testCase.want || !strings.Contains(check.Detail, testCase.detail) {
				t.Fatalf("got %+v", check)
			}
		})
	}
}

func TestAssess_NoLayoutSkips(t *testing.T) {
	check := statusOf(t, doctor.Assess(doctor.Facts{Config: &config.Config{}}), "layout")
	if check.Status != doctor.Skip {
		t.Fatalf("got %+v", check)
	}
}
