# focus_app cycles through an application's windows in desktop order

**Status:** accepted

`mimi action focus_app <app>` brings one of an application's windows to the
front, switching to its space first. Run again while that application is in
front, it moves to another of its windows. We decided that the window it moves
to is the next one in an order read off the desktop each time, by space and
then by the window server's number, and never the next one in a list mimi
remembers.

## Considered options

- **Most recently used order, like Cmd-Tab.** Rejected: the window server
  lists an application's windows front to back, so the "next" window is always
  the second most recent, and two presses only ever swap the same two windows.
  Reaching a third takes remembering which ones were visited, which is the
  option below.
- **Remember the cycle position.** Rejected: mimi's commands are one-shot and
  run on either of two paths, a CLI process that exits and a daemon that may or
  may not be listening. State that lives in one of them is invisible to the
  other, and the same hotkey would cycle differently depending on whether the
  daemon was up. `docs/ARCHITECTURE.md` treats that difference as a bug class.
- **Order by space, then by window number (chosen).** The space index walks
  the spaces left to right as Mission Control lays them out; the window number
  is assigned at creation and stable for the window's life, so within a space
  the order is by age. Both are read fresh on every press, so the cycle is the
  same from the CLI, from the daemon, and after either restarts.

## Consequences

- **The first press goes to the most recently used window.** From another
  application, the user wants the window they last left, and the window
  server's order says which that is. Only once the application is in front
  does the stable order apply, because only then is there a "next".
- **The cycle is predictable, not adaptive.** A user who bounces between two
  of five windows visits the other three on the way round. That is the price
  of an order that needs no memory, and `focus_window --same-app` remains the
  tool for stepping within one space.
- **The application's windows come from the window server, not from
  Accessibility.** An application's Accessibility window list holds only the
  windows on the active space; a window on another space, the one this action
  exists to reach, is simply absent from it. The window server lists every
  window, so the candidates are read there, narrowed to real windows by the
  same tags, attributes, parent and level tests yabai uses, and given their
  space by `SLSCopySpacesForWindows`. Those are private SkyLight calls with the
  usual caveat: they may break on a macOS update.
- **The switch comes before the raise.** Because Accessibility only lists a
  window once its space is in front, the action switches space first and only
  then looks the window up by number to raise it. The seam says so: an
  application's windows cross it as numbers, not handles, and raising takes a
  pid and a number.
- **Recognising the current window needs a stable identity.** The window in
  front is found through Accessibility, the application's windows through the
  window server, and the two are matched by the window server's number, which
  is the same however a window was reached. Windows cross the Desktop seam
  carrying that number for this reason.
- **The remote-token workaround was rejected.** yabai can build an
  Accessibility element for a window the list omits by sweeping element ids
  with `_AXUIElementCreateWithRemoteToken`. Measured here, a sweep that finds
  nothing runs 0.4s, and it runs on every press for any application with an
  auxiliary window the sweep cannot resolve. Switching first costs nothing.
