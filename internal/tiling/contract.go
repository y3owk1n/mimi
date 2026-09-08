package tiling

import (
	"encoding/json"

	"github.com/y3owk1n/mimi/internal/action"
)

// InputVersion is the version of the input a layout reads. It moves when a
// field is renamed, removed, or changes meaning, so a layout can refuse an
// input it was not written for; adding a field does not move it.
const InputVersion = 1

// Event is why the engine ran: the kind of the event that woke it, the
// application the event was about when it was about one, and for a command
// the user sent, its name and arguments.
type Event struct {
	Kind     string   `json:"kind"`
	App      string   `json:"app,omitempty"`
	BundleID string   `json:"bundleId,omitempty"`
	PID      int      `json:"pid,omitempty"`
	Name     string   `json:"name,omitempty"`
	Args     []string `json:"args,omitempty"`
}

// Input is everything the layout is told: the event, the active space, the
// displays and windows exactly as the queries report them, and the state the
// layout returned on its last run for this space, or null the first time.
type Input struct {
	Version  int                   `json:"version"`
	Event    Event                 `json:"event"`
	Space    int                   `json:"space"`
	Displays []action.DisplayEntry `json:"displays"`
	Focused  int                   `json:"focused"`
	Windows  []action.WindowEntry  `json:"windows"`
	State    json.RawMessage       `json:"state"`
}

// Output is what the layout prints back: the frames to apply, in the shape
// apply_frames takes, and the state to hand it next time. A layout that
// prints nothing at all is read as an empty Output, which changes nothing.
type Output struct {
	Frames []action.WindowFrame `json:"frames"`
	State  json.RawMessage      `json:"state"`
}

// Event kinds the engine reports beyond the hookable ones it forwards from
// the bus: a preview run by hand, a relayout asked for by name, a named
// command whose meaning is the layout's to decide, and the two passes the
// daemon's own lifecycle runs.
const (
	EventPreview  = "preview"
	EventRelayout = "relayout"
	EventCommand  = "command"
	// EventStartup is the pass the daemon runs as it starts with tiling
	// enabled, so the windows already open are laid out without waiting for
	// one of them to change.
	EventStartup = "startup"
	// EventReload is the pass a reload runs when it switches tiling on or
	// names another layout. A reload that changes neither runs none.
	EventReload = "reload"
)

// EventFromArgs is the tiling action's payload as the layout hears of it.
func EventFromArgs(args action.TilingArgs) Event {
	if args.Kind == action.TilingCommand {
		return Event{Kind: EventCommand, Name: args.Name, Args: args.Args}
	}

	return Event{Kind: EventRelayout}
}
