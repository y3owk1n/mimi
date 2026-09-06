package native //nolint:testpackage // tests unexported workspaceChangeEvent

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/events"
)

const (
	testWindowCount = 3
	testInfoJSON    = `{"windows":[]}`
)

// TestWorkspaceChangeEvent_CarriesTheActiveSpace pins what reaches a
// workspace hook: the window count and JSON it always carried, and now the
// space in front and the count, which is what lets a hook show a space number
// without asking anything else.
func TestWorkspaceChangeEvent_CarriesTheActiveSpace(t *testing.T) {
	t.Parallel()

	evt := workspaceChangeEvent(
		events.WorkspaceChanged,
		testWindowCount,
		testInfoJSON,
		func() (int, int, bool) { return 2, 5, true },
	)

	if evt.Kind != events.WorkspaceChanged {
		t.Fatalf("Kind = %q, want %q", evt.Kind, events.WorkspaceChanged)
	}

	want := map[string]string{
		"windows_count": "3",
		"info":          testInfoJSON,
		"space_index":   "2",
		"space_count":   "5",
	}

	for key, wantValue := range want {
		if got := evt.Extra[key]; got != wantValue {
			t.Errorf("Extra[%q] = %q, want %q", key, got, wantValue)
		}
	}

	if len(evt.Extra) != len(want) {
		t.Errorf("Extra = %v, want exactly %d keys", evt.Extra, len(want))
	}
}

// TestWorkspaceChangeEvent_OmitsTheSpaceItCannotResolve: a hook reads an
// unset mimi_SPACE_INDEX as "unknown", which is the truth; a zero would read
// as a space number.
func TestWorkspaceChangeEvent_OmitsTheSpaceItCannotResolve(t *testing.T) {
	t.Parallel()

	evt := workspaceChangeEvent(
		events.WorkspaceChanged,
		testWindowCount,
		"",
		func() (int, int, bool) { return 0, 0, false },
	)

	for _, key := range []string{"space_index", "space_count", "info"} {
		if got, ok := evt.Extra[key]; ok {
			t.Errorf("Extra[%q] = %q, want it absent", key, got)
		}
	}

	if got := evt.Extra["windows_count"]; got != "3" {
		t.Errorf("Extra[windows_count] = %q, want %q", got, "3")
	}
}
