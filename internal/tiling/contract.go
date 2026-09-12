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
	// Windows are the windows a window_move or window_resize event is
	// about: the ones the user dragged, by number, so a layout knows which
	// window moved or which edge was dragged without diffing frames itself.
	Windows []uint32 `json:"windows,omitempty"`
}

// Input is everything the layout is told for one display: the event, the
// display to fill and the space in front on it, the windows whose centers
// are on that display exactly as the queries report them, every display for
// reference, and the state the layout returned on its last run for this
// display and space, or null the first time.
//
// The engine runs the layout once per display that has windows, so a layout
// is written for one display and never sees another's windows. A window
// dragged to another display leaves one input and appears in the other on
// the next run.
type Input struct {
	Version int                 `json:"version"`
	Event   Event               `json:"event"`
	Display action.DisplayEntry `json:"display"`
	Space   int                 `json:"space"`
	// Gap is the space to leave between windows and at the display's
	// edges, in points: tiling.gap when the config sets it, else the macOS
	// tiled-window margin resize_window honors too, or 0 when that is
	// off. Resolved here so every layout uses the one number.
	Gap      float64               `json:"gap"`
	Displays []action.DisplayEntry `json:"displays"`
	Focused  int                   `json:"focused"`
	Windows  []action.WindowEntry  `json:"windows"`
	State    json.RawMessage       `json:"state"`

	// spaceID is which space Space names, as the window server identifies
	// it, and it is what the engine files this input's state under. It is
	// unexported because a layout has no use for it. Space is the number
	// the user counts, and this is the number that does not change when the
	// user reorders their spaces in Mission Control. It is 0 when the space
	// could not be resolved, and the engine then keeps no state at all.
	spaceID uint64
}

// Output is what the layout prints back: the frames to apply, in the shape
// apply_frames takes, and the state to hand it next time. A layout that
// prints nothing at all is read as an empty Output, which changes nothing.
type Output struct {
	Frames []action.WindowFrame `json:"frames"`
	State  json.RawMessage      `json:"state"`
	// Focus, when set, is the window the layout wants keyboard focus on,
	// given before the frames are applied: how a layout moves focus along
	// its own structure, a strip's next column say, where spatial focus
	// cannot.
	Focus uint32 `json:"focus,omitempty"`
	// Before is command lines the engine runs through settings.hook_shell
	// before the focus and the frames, all at once. It waits for every one,
	// each bounded by tiling.command_timeout_secs, so they have finished
	// when the frames move.
	Before []string `json:"before,omitempty"`
	// After is command lines the engine runs through settings.hook_shell
	// once the frames have been applied, and once the animation has ended
	// when one runs. They run detached from the pass, in order, each
	// bounded by tiling.command_timeout_secs, with their output dropped. A
	// layout uses it to act on the frames it returned, warping the cursor
	// to the focused window say, once they are known to be applied.
	After []string `json:"after,omitempty"`
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
