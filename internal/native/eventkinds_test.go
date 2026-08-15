package native //nolint:testpackage // tests unexported kindFromInt against the eventkinds.h fixture

// This file pins the one boundary the compiler cannot check: eventkinds.h's
// C enum and events/types.go's Go constants are two independently
// hand-maintained sources of truth, tied together only by a comment ("Keep
// in sync with events/types.go") in the header and by kindFromInt's switch
// in bridge.go. If a future edit renumbers a MIMI_KIND_* constant, renames
// an events.EventKind, or drops (or adds) a case in kindFromInt's switch
// without updating the other side, this test fails instead of the drift
// surfacing as a hook that silently never fires.
//
// It reads eventkinds.h as a plain text fixture rather than importing it via
// cgo: `go test` rejects `import "C"` in _test.go files outright ("use of
// cgo in test .go files not supported"), so a cgo-based version of this test
// cannot compile.

import (
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/y3owk1n/mimi/internal/events"
)

var defineRe = regexp.MustCompile(`(?m)^#define\s+(MIMI_KIND_\w+)\s+(\d+)\s*$`)

// parseEventKindsHeader extracts every "#define MIMI_KIND_X N" line from
// eventkinds.h into a name->value map.
func parseEventKindsHeader(t *testing.T) map[string]int {
	t.Helper()

	// go test runs with the package directory as the working directory, so
	// this relative path is stable regardless of the caller's cwd.
	data, err := os.ReadFile("eventkinds.h")
	if err != nil {
		t.Fatalf("reading eventkinds.h: %v", err)
	}

	matches := defineRe.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		t.Fatal("eventkinds.h: found no #define MIMI_KIND_* lines; parser regex may be stale")
	}

	defines := make(map[string]int, len(matches))

	for _, m := range matches {
		name, rawValue := m[1], m[2]

		value, err := strconv.Atoi(rawValue)
		if err != nil {
			t.Fatalf("eventkinds.h: %s has non-numeric value %q: %v", name, rawValue, err)
		}

		if _, dup := defines[name]; dup {
			t.Fatalf("eventkinds.h: %s is defined more than once", name)
		}

		defines[name] = value
	}

	return defines
}

// mappedKinds are the eventkinds.h symbols that kindFromInt routes to an
// events.EventKind, keyed by the header's own symbol name so a mismatch
// names the constant that drifted.
var mappedKinds = map[string]events.EventKind{
	"MIMI_KIND_APP_ACTIVATE":        events.AppActivate,
	"MIMI_KIND_APP_DEACTIVATE":      events.AppDeactivate,
	"MIMI_KIND_APP_LAUNCH":          events.AppLaunch,
	"MIMI_KIND_APP_QUIT":            events.AppQuit,
	"MIMI_KIND_APP_HIDE":            events.AppHide,
	"MIMI_KIND_APP_UNHIDE":          events.AppUnhide,
	"MIMI_KIND_WINDOW_FOCUS":        events.WindowFocus,
	"MIMI_KIND_WINDOW_TITLE_CHANGE": events.WindowTitleChange,
	"MIMI_KIND_WINDOW_CREATED":      events.WindowCreated,
	"MIMI_KIND_WINDOW_CLOSED":       events.WindowClosed,
	"MIMI_KIND_WINDOW_RESIZING":     events.WindowResizing,
	"MIMI_KIND_WORKSPACE_CHANGED":   events.WorkspaceChanged,
}

// unmappedKinds are eventkinds.h symbols that are deliberately NOT part of
// the hookable events.EventKind set kindFromInt produces (power/session/
// volume/appearance events, handled outside the hook-eligible event path).
// A symbol missing from both this set and mappedKinds fails
// TestEventKindsHeader_EveryDefineIsAccountedFor, forcing a conscious choice
// when a new C kind is added.
var unmappedKinds = map[string]struct{}{
	"MIMI_KIND_WILL_SLEEP":         {},
	"MIMI_KIND_DID_WAKE":           {},
	"MIMI_KIND_SESSION_RESIGN":     {},
	"MIMI_KIND_SESSION_BECOME":     {},
	"MIMI_KIND_WILL_POWER_OFF":     {},
	"MIMI_KIND_VOLUME_MOUNT":       {},
	"MIMI_KIND_VOLUME_UNMOUNT":     {},
	"MIMI_KIND_APPEARANCE_CHANGED": {},
}

func TestEventKindsHeader_MappedConstantsAgreeWithKindFromInt(t *testing.T) {
	t.Parallel()

	defines := parseEventKindsHeader(t)

	for name, want := range mappedKinds {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			value, ok := defines[name]
			if !ok {
				t.Fatalf("eventkinds.h has no #define for %s", name)
			}

			if got := kindFromInt(value); got != want {
				t.Errorf("kindFromInt(%s=%d) = %q, want %q", name, value, got, want)
			}
		})
	}
}

func TestEventKindsHeader_EveryHookableKindHasAConstant(t *testing.T) {
	t.Parallel()

	// Every hookable events.EventKind must have a matching header constant
	// wired through kindFromInt -- a Go-side addition with no MIMI_KIND_*
	// counterpart would otherwise never be reachable from a real event.
	//
	// The one deliberate exception is events.WindowResize: it is never
	// produced by kindFromInt. internal/observe/router.go debounces the raw
	// MIMI_KIND_WINDOW_RESIZING stream (-> events.WindowResizing) and
	// synthesizes WindowResize itself once resizing settles, so it has no
	// C-side constant to agree with.
	for _, kind := range events.AllKinds {
		if kind == events.WindowResize {
			continue
		}

		found := false

		for _, mapped := range mappedKinds {
			if mapped == kind {
				found = true

				break
			}
		}

		if !found {
			t.Errorf("events.EventKind %q has no eventkinds.h constant in mappedKinds", kind)
		}
	}
}

func TestEventKindsHeader_EveryDefineIsAccountedFor(t *testing.T) {
	t.Parallel()

	defines := parseEventKindsHeader(t)

	for name := range defines {
		_, isMapped := mappedKinds[name]
		_, isUnmapped := unmappedKinds[name]

		if !isMapped && !isUnmapped {
			t.Errorf(
				"eventkinds.h defines %s, which is neither in mappedKinds nor unmappedKinds in this test -- "+
					"add it to whichever set matches whether kindFromInt should route it to an events.EventKind",
				name,
			)
		}

		if isMapped && isUnmapped {
			t.Errorf("%s appears in both mappedKinds and unmappedKinds", name)
		}
	}

	for name := range mappedKinds {
		if _, ok := defines[name]; !ok {
			t.Errorf("mappedKinds references %s, which eventkinds.h no longer defines", name)
		}
	}

	for name := range unmappedKinds {
		if _, ok := defines[name]; !ok {
			t.Errorf("unmappedKinds references %s, which eventkinds.h no longer defines", name)
		}
	}
}

func TestEventKindsHeader_UnmappedConstantsReturnUnknown(t *testing.T) {
	t.Parallel()

	defines := parseEventKindsHeader(t)

	for name := range unmappedKinds {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			value, ok := defines[name]
			if !ok {
				t.Fatalf("eventkinds.h has no #define for %s", name)
			}

			if got := kindFromInt(value); got != events.EventKind("unknown") {
				t.Errorf("kindFromInt(%s=%d) = %q, want %q (not yet wired to an events.EventKind)",
					name, value, got, events.EventKind("unknown"))
			}
		})
	}
}
