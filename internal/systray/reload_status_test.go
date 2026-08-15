//nolint:testpackage // pins the last-reload status, an unexported seam of the tray component
package systray

import (
	"context"
	"testing"
	"time"
)

// The clock readings the tests render. Fixed, because the status line is
// absolute local time and a test that waited for a real one would be pinning
// nothing.
const (
	testReloadHour   = 14
	testReloadMinute = 32
)

// fixedClock reads a fixed wall-clock time, standing in for time.Now. The
// zone is pinned rather than local because what these tests assert is the
// rendered reading of whatever clock the component was given; the daemon's own
// clock is the local one, and it is time.Now that supplies it.
func fixedClock(hour, minute int) func() time.Time {
	return func() time.Time {
		return time.Date(2026, time.August, 15, hour, minute, 0, 0, time.UTC)
	}
}

// newStatusTestComponent builds a component with a fixed clock and no menu —
// the state the daemon's callback finds during startup, before Cocoa is ready.
func newStatusTestComponent(t *testing.T) *Component {
	t.Helper()

	requestReload := func(context.Context, string) error { return nil }

	component := NewComponent("v0", "config.toml", requestReload, func() {}, false, nil)
	component.now = fixedClock(testReloadHour, testReloadMinute)

	t.Cleanup(component.Close)

	return component
}

// newStatusTestItem stands in for the item OnReady adds. Its id matches no
// item in Cocoa's menu, so titling and disabling it change only the Go-side
// copy a test reads.
func newStatusTestItem() *MenuItem {
	return &MenuItem{ClickedCh: make(chan struct{}, 1)}
}

// TestFormatReloadStatus_RendersEveryOutcome pins the line the menu shows for
// each reload outcome. The three outcomes must stay distinguishable at a
// glance: a reload that left restart-only settings unapplied worked and still
// did not do what the user asked, so rendering it as a plain success would be
// the same lie the reload menu item stopped telling
// (docs/adr/0002-reload-is-signal-mediated.md).
//
// The time is absolute and local. A relative "2 min ago" would need a timer
// ticking behind a label read only when someone opens the menu.
func TestFormatReloadStatus_RendersEveryOutcome(t *testing.T) {
	t.Parallel()

	reloadedAt := fixedClock(testReloadHour, testReloadMinute)()

	tests := []struct {
		name   string
		status *reloadStatus
		want   string
	}{
		{
			name:   "no reload has happened yet",
			status: nil,
			want:   "No config reload yet",
		},
		{
			name:   "the reload applied cleanly",
			status: &reloadStatus{outcome: ReloadOutcomeApplied, at: reloadedAt},
			want:   "Reloaded 14:32",
		},
		{
			name:   "the reload left restart-only settings unapplied",
			status: &reloadStatus{outcome: ReloadOutcomeRestartRequired, at: reloadedAt},
			want:   "Reloaded 14:32 — restart required",
		},
		{
			name:   "the reload failed",
			status: &reloadStatus{outcome: ReloadOutcomeFailed, at: reloadedAt},
			want:   "Reload failed 14:32",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := formatReloadStatus(testCase.status)
			if got != testCase.want {
				t.Errorf("formatReloadStatus() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestComponent_ReportReload_UpdatesAMenuThatAlreadyExists pins the ordinary
// case: the menu is up, and a reload from any trigger — a file save or a
// signal, not only a click on the menu item — rewrites the status line, with
// the time it happened.
func TestComponent_ReportReload_UpdatesAMenuThatAlreadyExists(t *testing.T) {
	t.Parallel()

	component := newStatusTestComponent(t)
	item := newStatusTestItem()

	component.adoptReloadStatusItem(item)

	component.ReportReload(ReloadOutcomeRestartRequired)

	got := item.Title()
	if got != "Reloaded 14:32 — restart required" {
		t.Errorf("status item title = %q, want %q", got, "Reloaded 14:32 — restart required")
	}
}

// TestComponent_ReportReload_IsRenderedWhenTheMenuIsBuiltLater pins that a
// reload landing before the menu exists is not lost. The daemon's core starts
// after the component is built but before Cocoa calls OnReady, so the first
// reload can be reported into a component that has no menu items at all; the
// component holds the outcome and renders it when it is handed the item.
func TestComponent_ReportReload_IsRenderedWhenTheMenuIsBuiltLater(t *testing.T) {
	t.Parallel()

	component := newStatusTestComponent(t)

	component.ReportReload(ReloadOutcomeFailed)

	item := newStatusTestItem()
	component.adoptReloadStatusItem(item)

	got := item.Title()
	if got != "Reload failed 14:32" {
		t.Errorf("status item title = %q, want %q", got, "Reload failed 14:32")
	}
}

// TestComponent_AdoptReloadStatusItem_DisablesTheLine pins that the status is a
// label and not a second way to ask for a reload. The systray reaches the
// reload path through exactly one item, and that item signals
// (docs/adr/0002-reload-is-signal-mediated.md).
func TestComponent_AdoptReloadStatusItem_DisablesTheLine(t *testing.T) {
	t.Parallel()

	component := newStatusTestComponent(t)
	item := newStatusTestItem()

	component.adoptReloadStatusItem(item)

	if !item.Disabled() {
		t.Error("status item is enabled, want disabled — it is a label, not a command")
	}
}
