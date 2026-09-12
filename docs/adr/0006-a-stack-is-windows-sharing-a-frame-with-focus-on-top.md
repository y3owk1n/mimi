# A stack is windows sharing one frame, and the one seen is the one with focus

**Status:** accepted

A layout can put several windows in one place so that only one is seen at a
time, the way yabai stacks and niri tabs. We decided that mimi changes no
z-order to arrange this, that the window seen is whichever holds keyboard
focus, and that a layout names its stacks only so mimi can mark them.

## The measurement that decided it

The obvious design is for the engine to raise the member the layout names,
leaving focus wherever the user left it. It cannot, and this was tested rather
than assumed.

`SLSOrderWindow(cid, wid, mode, relativeTo)` is the private SkyLight call that
orders one window against another. mimi already links it and already uses it,
on its own border overlay (`internal/native/border.m`). Run against a window
belonging to another application, from mimi's own connection, every form of it
fails:

| Call | Result |
| --- | --- |
| order the front window below the second | `kCGErrorFailure` (1000), no change |
| order the second window above the front | `kCGErrorFailure` (1000), no change |
| order either relative to nothing | `kCGErrorFailure` (1000), no change |
| `SLSSetWindowLevel` on a foreign window | returned 0, no change |

The window server accepts these only from a connection with the scripting
addition loaded, which needs SIP disabled. mimi's whole premise is that it
does not ask for that.

The only raise mimi has is `MimiRaiseWindowNumber`
(`internal/native/application.m`), which goes through `MimiActivateWindow` and
so performs `activateWithOptions:`, sets `kAXMain` and `kAXFocused`, and only
then `kAXRaiseAction`. There is no way to reach the last step alone.

## Considered options

- **The engine raises the active member after the frames.** Rejected on the
  measurement above: the only raise available also takes focus, so a stack on
  a display the user was not looking at would steal their keyboard every pass.
- **Minimise the members that are not active.** Rejected: a minimised window
  leaves the window list mimi and every layout read, so the stack would
  dissolve on the next pass, and the Dock animates every change.
- **Move the inactive members off screen.** Rejected: it works, and it makes a
  mess of everything else. Those windows would be on another display as far as
  the layout is concerned, Mission Control would show them adrift, and an
  application that saves its frame would reopen off screen.
- **Same frame, focus decides, and mark it (chosen).** Identical geometry
  means the window in front covers the rest exactly. Focus already moves the
  front window to the front, which the window server reports as each window's
  `order`. Nothing needs a private call that does not work.

## Consequences

- **A layout switches members with the `focus` key it already had.** There is
  no new verb for moving within a stack, because moving focus is the whole
  mechanism. `stacked.py` answers `next` and `prev` by returning `focus`.
- **`active` is the layout's own reckoning, not a reading of the desktop.**
  The two can differ the moment the user presses Cmd-Tab, which surfaces a
  buried member without telling the layout. The input reports what is really
  in front, through `focused` and each window's `order`, so a layout that
  cares can reconcile on the next pass. mimi does not reconcile for it, since
  which of the two is right is the layout's business.
- **The indicator is the feature.** Windows sharing a frame was always
  possible and always invisible. `[tiling.stackbar]` draws one segment per
  member along the top of the shared frame, so a stack of four does not look
  like a window of one. A layout that names no stack has nothing marked.
- **The engine refuses a stack it cannot draw.** Fewer than two windows is
  every window in every layout, and a member with no frame in the same output
  would put a mark where nothing is. Either is dropped with a debug line, and
  the frames still apply, which is the containment the rest of the engine
  uses.
- **A real `mimi action lower_window` stays impossible.** The same measurement
  rules it out, so the z-order verb considered alongside this is not a matter
  of implementing it, and will not become one unless macOS changes.
