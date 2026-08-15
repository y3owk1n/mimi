package systray

import "time"

// reloadStatusTimeFormat renders the local wall-clock time a reload happened.
// It is absolute on purpose: a relative "2 min ago" would need a timer ticking
// behind a label that is read only when someone opens the menu.
const reloadStatusTimeFormat = "15:04"

// ReloadOutcome is what a reload did, as the daemon that ran it saw it. The
// systray is told the outcome after the fact and holds it; it never learns of
// one by asking, and gains no route into the reload path from having it
// (docs/adr/0002-reload-is-signal-mediated.md).
type ReloadOutcome int

// The outcomes count from one so that the zero value is none of them: an
// outcome nobody set renders as unknown rather than as the success that
// happens to sort first.
const (
	// ReloadOutcomeApplied is a reload the daemon applied in full.
	ReloadOutcomeApplied ReloadOutcome = iota + 1
	// ReloadOutcomeRestartRequired is a reload the daemon applied as far as it
	// can: restart-only settings changed too, and those keep their old values
	// until the daemon is restarted. It is its own outcome rather than a
	// success because it is the case where the reload worked and the user's
	// change still did nothing.
	ReloadOutcomeRestartRequired
	// ReloadOutcomeFailed is a reload the daemon did not apply at all; the
	// previous config is still running, entirely.
	ReloadOutcomeFailed
)

// reloadStatus is the last reload the daemon reported, if there was one.
type reloadStatus struct {
	outcome ReloadOutcome
	at      time.Time
}

// ReportReload records the outcome of the daemon's most recent reload and
// updates the menu to match. It is the systray's whole share of reloading:
// one call, one way, after the fact — every reload trigger reaches it, because
// the daemon reports from the single place all of them funnel through, so a
// reload from a file save or a signal shows up here just as a click does.
//
// It is safe to call before Cocoa has built the menu, which is the normal case
// for a reload that lands during startup: the outcome is held as state and
// OnReady renders it.
func (c *Component) ReportReload(outcome ReloadOutcome) {
	c.reloadMu.Lock()
	defer c.reloadMu.Unlock()

	c.lastReload = &reloadStatus{outcome: outcome, at: c.now()}

	if c.mReloadStatus != nil {
		// SetTitle marshals onto the main thread, so reporting from the
		// daemon's reload goroutine is safe.
		c.mReloadStatus.SetTitle(formatReloadStatus(c.lastReload))
	}
}

// adoptReloadStatusItem takes ownership of the menu item that shows the reload
// status, giving it the outcome the component is already holding. Taking the
// item and rendering into it happen under one lock so that a reload landing
// while Cocoa builds the menu cannot slip between the two and leave the line
// showing an outcome older than the one recorded.
//
// The item is disabled because it is a label: it reports what a reload did and
// offers no way to ask for another one.
func (c *Component) adoptReloadStatusItem(item *MenuItem) {
	c.reloadMu.Lock()
	defer c.reloadMu.Unlock()

	c.mReloadStatus = item

	item.SetTitle(formatReloadStatus(c.lastReload))
	item.Disable()
}

// formatReloadStatus renders a reload status for the menu. A nil status is a
// daemon that has not reloaded since it started, which says so rather than
// showing a time or implying a success that never happened.
//
// Nothing from the user's config file can reach these lines: the outcome is
// three values and a clock reading, so no setting name, no setting value and
// no error text has a way in.
func formatReloadStatus(status *reloadStatus) string {
	if status == nil {
		return "No config reload yet"
	}

	when := status.at.Format(reloadStatusTimeFormat)

	switch status.outcome {
	case ReloadOutcomeApplied:
		return "Reloaded " + when
	case ReloadOutcomeRestartRequired:
		return "Reloaded " + when + " — restart required"
	case ReloadOutcomeFailed:
		return "Reload failed " + when
	default:
		// An outcome nobody set, or one added without a line of its own.
		// Honest rather than claiming a success it knows nothing about.
		return "Reload reported " + when + " — unknown outcome"
	}
}
