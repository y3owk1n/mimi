package baseline_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/baseline"
)

// The counts the recorder covers: ten presets in two margin states plus one
// system-default case, nine anchors in two margin states, ten explicit-flag
// combinations, and four focus directions.
const (
	wantResizeCases = 10*2 + 1 + 9*2 + 10
	wantFocusCases  = 4
)

func TestLoad_ReturnsRecordedCases(t *testing.T) {
	recording, err := baseline.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(recording.Resize) != wantResizeCases {
		t.Errorf("Load() returned %d resize cases, want %d", len(recording.Resize), wantResizeCases)
	}

	if len(recording.Focus) != wantFocusCases {
		t.Errorf("Load() returned %d focus cases, want %d", len(recording.Focus), wantFocusCases)
	}

	if recording.Display.Visible.W <= 0 || recording.Display.Visible.H <= 0 {
		t.Errorf("Load() returned an empty visible frame %s", recording.Display.Visible)
	}
}

func TestLoad_CasesAreUsableAsFixtures(t *testing.T) {
	recording, err := baseline.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	seen := map[string]bool{}

	for _, resize := range recording.Resize {
		if resize.Name == "" {
			t.Error("a resize case has no name")
		}

		if seen[resize.Name] {
			t.Errorf("resize case %q is recorded twice", resize.Name)
		}

		seen[resize.Name] = true

		if len(resize.Args) == 0 {
			t.Errorf("resize case %q has no args", resize.Name)
		}

		if resize.Want.W <= 0 || resize.Want.H <= 0 {
			t.Errorf("resize case %q recorded an empty frame %s", resize.Name, resize.Want)
		}
	}

	for _, focus := range recording.Focus {
		if len(focus.Arrangement) == 0 {
			t.Errorf("focus case %q recorded no arrangement", focus.Direction)
		}

		if focus.Current < 0 || focus.Current >= len(focus.Arrangement) {
			t.Errorf(
				"focus case %q has current index %d out of range",
				focus.Direction,
				focus.Current,
			)
		}

		if focus.Want < 0 || focus.Want >= len(focus.Arrangement) {
			t.Errorf("focus case %q has target index %d out of range", focus.Direction, focus.Want)
		}

		if focus.Want == focus.Current {
			t.Errorf("focus case %q did not move focus", focus.Direction)
		}
	}
}

func TestRecording_LookupsFindRecordedCases(t *testing.T) {
	recording, err := baseline.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(recording.Resize) == 0 || len(recording.Focus) == 0 {
		t.Fatal("the recording holds no cases to look up")
	}

	first := recording.Resize[0]

	gotResize, found := recording.ResizeCase(first.Name)
	if !found || gotResize.Name != first.Name {
		t.Errorf("ResizeCase(%q) = %v, %v; want the recorded case", first.Name, gotResize, found)
	}

	_, found = recording.ResizeCase("no-such-case")
	if found {
		t.Error("ResizeCase() found a case that was never recorded")
	}

	firstFocus := recording.Focus[0]

	gotFocus, found := recording.FocusCase(firstFocus.Direction)
	if !found || gotFocus.Direction != firstFocus.Direction {
		t.Errorf(
			"FocusCase(%q) = %v, %v; want the recorded case",
			firstFocus.Direction,
			gotFocus,
			found,
		)
	}

	_, found = recording.FocusCase("no-such-direction")
	if found {
		t.Error("FocusCase() found a case that was never recorded")
	}
}

func TestEncode_RoundTripsThroughJSON(t *testing.T) {
	recording, err := baseline.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	data, err := baseline.Encode(recording)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Error("Encode() did not end the recording with a newline")
	}
}
