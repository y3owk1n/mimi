# Layout contract reference

This is the field-by-field reference for the JSON a layout program reads and
prints. [TILING.md](TILING.md) is the guide, with examples and the shipped
layouts. This page answers what each field is, when it is present, and what
mimi does with what you print. It matches the Go types in
`internal/tiling/contract.go` and `internal/action/windows_query.go`, which
are the source of truth when the two disagree.

## Framing

A layout is a command line run through `settings.hook_shell` with `-c`. mimi
sets no environment variables of its own for it and no working directory, so
it inherits the daemon's.

| Mode | Input | Output | Process |
| --- | --- | --- | --- |
| `layout_mode = "oneshot"` (default) | One JSON document on stdin, then stdin closes | One JSON document on stdout, read once the process exits | One process per pass |
| `layout_mode = "resident"` | One JSON document per line on stdin, for as long as stdin stays open | One JSON document per line on stdout, flushed after each | One process, started on the first pass and kept |

Rules that hold in both modes:

- mimi reads a blank stdout, or in resident mode a blank line, as an empty
  output. It changes nothing and keeps the previous state.
- mimi ignores unknown fields in the output. A field of the wrong type is a
  decoding failure.
- mimi keeps up to 4096 bytes of stderr and quotes it in the error when the
  program fails.
- `tiling.timeout_secs`, default 5, bounds one pass. In resident mode it
  bounds one exchange, not the process.
- mimi reads at most 8 MiB of one resident output line.

What a failure does:

| The program | mimi |
| --- | --- |
| Exits non-zero (oneshot) | Logs `tiling pass failed` at warn with the stderr tail. Applies nothing. |
| Prints something that is not the output shape | Same. The error names the decoding problem. |
| Runs past the timeout | Kills it and logs the same. |
| Exits, in resident mode | Logs it, and the next pass starts it again. After a failure in a row the restart waits 100 ms, doubling up to 5 s. |

The next event tries again. Nothing a failed pass printed is kept, and no
frame from a failed pass is applied.

## Input

One input per display that has a window on it and is not showing a
full-screen space. Every input in one pass carries the same `event` and
`displays`.

| Field | Type | Present | Meaning |
| --- | --- | --- | --- |
| `version` | integer | always | `1`. Moves only when a field is renamed, removed, or changes meaning. A field added does not move it. |
| `event` | object | always | Why this pass runs. See [Event](#event). |
| `display` | display | always | The display this input is for. Place windows inside its `visible`. |
| `space` | integer | always | The 1-based Mission Control index of the space in front on that display. |
| `gap` | number | always | Points to leave between windows and at the display's edges: `tiling.gap` when set, else the macOS tiled-window margin when that is on, else `0`. |
| `displays` | array of display | always | Every connected display, in `move_window_to_display` order. |
| `focused` | integer | always | Index into `windows` of the window with keyboard focus, or `-1` when no window on this display has it. |
| `windows` | array of window | always | The windows on this display, in `focus_window` order. Never empty, since a display with no window gets no input. |
| `state` | any JSON | always | What you printed as `state` on the last pass for this display and space, or `null` when there is none. |
| `unmanaged` | array of integer | when non-empty | Window numbers on this display you named in `unmanaged` on an earlier pass and have not claimed back since. |
| `stacks` | array of stack | when non-empty | The stacks you named on the last pass for this display that mimi kept. See [Stack](#stack). |

### Event

| Field | Type | Present | Meaning |
| --- | --- | --- | --- |
| `kind` | string | always | One of the kinds below. |
| `app` | string | some kinds | The localized name of the application the event is about. |
| `bundleId` | string | some kinds | Its bundle identifier. |
| `pid` | integer | some kinds | Its process id. |
| `name` | string | `command` | The word after `mimi tiling cmd`. |
| `args` | array of string | `command`, when any | The words after that. |
| `windows` | array of integer | `window_move`, `window_resize` | The numbers of the windows the user dragged. |
| `modifiers` | array of string | `window_move`, `window_resize`, when any | The modifier keys held during the drag, from `shift`, `control`, `option`, `command`, in that order. mimi reads the keys while the button is down and reports what was held last. |

`app`, `bundleId` and `pid` come from the hook event that woke the pass and
are present when that event named an application. mimi never puts a window
title in the input event.

| `kind` | Sent when | Carries |
| --- | --- | --- |
| `app_activate`, `app_hide`, `app_unhide`, `app_quit` | The hook event of that name | `app`, `bundleId`, `pid` |
| `window_created`, `window_closed`, `window_focus`, `window_minimize`, `window_unminimize` | The hook event of that name | `app`, `bundleId`, `pid` |
| `workspace_changed` | The space in front changed on any display. Entering or leaving full screen also fires it | none |
| `display_changed` | A display was connected, disconnected, or rearranged | none |
| `_ax_attached` | The daemon's Accessibility observer reached an application it could not observe when it launched | `app`, `bundleId`, `pid` |
| `startup` | The daemon started with tiling on | none |
| `reload` | A reload switched tiling on or named another layout | none |
| `preview` | `mimi tiling preview` | none |
| `relayout` | `mimi tiling relayout` | none |
| `command` | `mimi tiling cmd <name> [args...]` | `name`, `args` |
| `window_move` | With `relayout_on_drag`, the user released a drag that moved a window more than it resized it | `windows`, `modifiers` |
| `window_resize` | With `relayout_on_drag`, the user released a drag that resized a window at least as much as it moved it | `windows`, `modifiers` |

The drop zone, while a drag is held, runs the layout with the same
`window_move` or `window_resize` event the release would send, and applies
nothing, keeps no state, and runs no `before` or `after` line. It only draws
the frame the layout gave the dragged window.

A `window_created` pass waits up to half a second for the window server to
list the new window before the input is built, so the window is in
`windows` when the layout runs.

### Display

The same object `mimi query displays` prints.

| Field | Type | Meaning |
| --- | --- | --- |
| `index` | integer | 1-based, in `move_window_to_display` order, left to right. |
| `id` | integer | The display identifier macOS uses. Stable while the display stays connected. |
| `frame` | frame | The whole display. |
| `visible` | frame | The display less the menu bar and the Dock. This is the area to fill. |

### Frame

| Field | Type | Meaning |
| --- | --- | --- |
| `x`, `y` | number | The top-left corner, in window coordinates. |
| `width`, `height` | number | The size, in points. |

Window coordinates put the origin at the top-left corner of the primary
display with y growing downward. A display above the primary has a negative
`y`. `mimi action resize_window --x --y` takes the same system.

### Window

The same object `mimi query windows` prints, plus `minSize`.

| Field | Type | Present | Meaning |
| --- | --- | --- | --- |
| `number` | integer | always | The window server's number. Stable for the window's lifetime, and how the output names a window. |
| `pid` | integer | always | The owning process. |
| `app` | string | always | The owner's localized name, `""` when macOS does not report one. |
| `bundleId` | string | always | The owner's bundle identifier, `""` when none. |
| `title` | string | always | The window's title, `""` when none. |
| `frame` | frame | always | Where the window is now. |
| `order` | integer | always | Its place in the stacking order among the desktop's windows, `0` for the one in front. Numbers compare but need not start at 0 or run without gaps. |
| `space` | integer | always | The 1-based index of the space it is on, `0` for a window assigned to every space. |
| `display` | integer | always | The `index` of the display holding its centre, which in an input is always `display.index`. |
| `minSize` | object | when learned | The smallest size the window has accepted. `width` and `height` in points, `0` on an axis the window took as asked. Give the window at least this. |

`windows` is ordered by position, the order `focus_window` cycles. `order`
answers which window is on top, which is a different question.

`minSize` is learned, not asked for. macOS gives no way to read an
application's minimum, so mimi remembers a window that was written smaller
than it landed. The learned minimums are kept across restarts and shown by
`mimi tiling state`.

### Stack

| Field | Type | Meaning |
| --- | --- | --- |
| `windows` | array of integer | The members, by number. |
| `active` | integer | The member the layout means to be seen. |

## Output

Every field is optional. An empty object, or nothing at all, changes nothing.

| Field | Type | mimi does |
| --- | --- | --- |
| `frames` | array of placement | Applies every one in a single write, after `before` and `focus`. A window not named keeps its place this pass and stays managed. |
| `state` | any JSON | Keeps it for the next pass on this display and space. Omit the key to keep the previous state. Print `null` to clear it. mimi keeps at most 64 display-and-space states and drops the one written longest ago past that. |
| `focus` | integer | Gives that window keyboard focus before the frames move. A window that cannot be focused logs at debug and the frames still apply. |
| `unmanaged` | array of integer | Stops watching those windows: a drag of one raises no pass and shows no drop zone, unless a modifier key is held, and they are handed back in the input's `unmanaged`. mimi reclassifies only the windows this input listed, so a run for one display never speaks for another's. A window stays unmanaged until a later run for the same display leaves it out. |
| `stacks` | array of stack | Keeps them for the stack bar and hands them back in the input. mimi drops a stack that names fewer than two windows, or a window with no frame in this output, and the frames still apply. |
| `before` | array of string | Runs every line through `settings.hook_shell -c` at once, before `focus` and the frames, and waits for all of them. mimi kills each past `tiling.command_timeout_secs`, default 1. A failure logs at debug and the frames still apply. |
| `after` | array of string | Runs the lines in order once the frames are applied, and once the animation has ended when one runs, detached from the pass. mimi kills each past `tiling.command_timeout_secs`, drops their output, and logs a failure at debug. |
| `target` | target | While a drag is held, marks that window in the drop zone. A pass ignores it. See [Target](#target). |

### Placement

| Field | Type | Present | Meaning |
| --- | --- | --- | --- |
| `number` | integer | required | The window to place. |
| `frame` | frame | required | Where to put it, in window coordinates. |
| `animate` | boolean | optional | With `[tiling.animation]` on, `false` places this window at once while the rest animate. Ignored when animation is off. |

mimi places a window the user just dragged at once whatever `animate` says,
so it does not slide away from under the pointer.

### Target

| Field | Type | Present | Meaning |
| --- | --- | --- | --- |
| `window` | integer | required | The window the drop acts on, other than the dragged one. A target naming the dragged window, or a window not in the input, marks nothing. |
| `action` | string | optional | The layout's word for what the drop does to it, `swap` or `insert` say. mimi passes it through and gives it no meaning. |

Only a `window_move` or `window_resize` output is read for it, and only by
the drop zone while the button is down. The drop zone draws the target
window's frame as it is on screen now, in `tiling.dropzone.target_color`
and `target_outline_color`, next to the zone for the dragged window.

### What a pass does with several outputs

One pass runs the layout once per display and reads every output before it
touches the desktop. mimi applies the frames from every display in one write.
`focus` from the last display in order that set one wins. `before` lines from
every display run together, and `after` lines run in display order.

If the space in front changed on any display while the layout ran, the pass
applies nothing, because the change itself raises the event that lays the new
space out.

## Versioning

`version` is `1`. mimi moves it when a field is renamed, removed, or changes
meaning, so a layout can refuse an input it was not written for. A field
added, in the input or in the output, does not move it. A layout that reads
only the fields it needs keeps working across additions.
