package native //nolint:testpackage // spaceHistory is unexported

import "testing"

func TestSpaceHistory_RecentIsTheSpaceBeforeTheLastSwitch(t *testing.T) {
	t.Parallel()

	steps := []struct {
		name string
		note uint64
		want uint64
	}{
		{name: "nothing to go back to before the first switch", note: 0, want: 0},
		{name: "the first switch remembers where it started", note: 2, want: 1},
		{name: "the same switch reported again changes nothing", note: 2, want: 1},
		{name: "going back toggles", note: 1, want: 2},
		{name: "and toggles again", note: 2, want: 1},
		{name: "a third space takes over", note: 3, want: 2},
	}

	var tracked spaceHistory

	tracked.start(1)

	for _, step := range steps {
		tracked.note(step.note)

		recent, watching := tracked.last()
		if !watching || recent != step.want {
			t.Fatalf("%s: last() = %d, %v, want %d, true", step.name, recent, watching, step.want)
		}
	}
}

func TestSpaceHistory_NotWatchingUntilStarted(t *testing.T) {
	t.Parallel()

	var tracked spaceHistory

	tracked.note(4)

	if _, watching := tracked.last(); watching {
		t.Fatal("last() reports watching before start, want false")
	}
}

func TestSpaceHistory_HoldSkipsTheSpacesASwitchPassesThrough(t *testing.T) {
	t.Parallel()

	var tracked spaceHistory

	tracked.start(1)
	tracked.hold()
	tracked.note(2)
	tracked.note(3)
	tracked.release()
	tracked.note(4)

	if recent, _ := tracked.last(); recent != 1 {
		t.Fatalf("last() = %d after a switch from 1 to 4 through 2 and 3, want 1", recent)
	}
}

func TestSpaceHistory_CountsEverySwitchItRecords(t *testing.T) {
	t.Parallel()

	var tracked spaceHistory

	tracked.start(1)
	tracked.note(2)
	tracked.note(2)
	tracked.hold()
	tracked.note(3)
	tracked.release()
	tracked.note(1)

	if got := tracked.switchCount(); got != 2 {
		t.Fatalf("switchCount() = %d after switches to 2 and back to 1, want 2", got)
	}
}
