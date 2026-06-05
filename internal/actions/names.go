package actions

// Names of the user-facing actions exposed by this package. These
// are the names that appear in CLI invocations and (in the
// future) as references from hook shell commands.
const (
	// NameFocusWindow cycles focus through focusable windows on the
	// active space. Accepts an optional --backward flag.
	NameFocusWindow = "focus_window"

	// NameSpace focuses a Mission Control space by its 1-based
	// index. Requires a single positional argument.
	NameSpace = "space"

	// NameMoveWindowToSpace moves the current focused window to a
	// Mission Control space by its 1-based index. Requires a
	// single positional argument.
	NameMoveWindowToSpace = "move_window_to_space"
)
