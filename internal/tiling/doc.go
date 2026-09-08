// Package tiling runs a user-supplied layout program against the desktop.
//
// mimi ships no layout. The engine here watches the event bus for window
// changes, settles a burst of them into one pass, and on each pass hands the
// layout everything on the active space as JSON: the event that woke it, the
// displays, the windows with their frames, and whatever state the layout
// returned last time for that space. The layout prints the frames to apply
// and its new state; the engine applies the frames and keeps the state.
//
// The contract the layout sees is Input and Output, versioned by
// InputVersion. It is the same JSON `mimi query windows`, `mimi query
// displays` and `mimi action apply_frames` speak, so a layout that works
// against those by hand works under the engine unchanged.
package tiling
