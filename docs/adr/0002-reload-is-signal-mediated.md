# Reload is mediated by SIGHUP

**Status:** accepted

A running daemon can be asked to reload its config from four places: the config
file watcher, a `SIGHUP` sent by hand, `mimi config reload`, and the systray's
Reload Config menu item. Only the watcher reaches the reload path in-process —
the CLI signals the daemon's pid, and the systray signals *itself*, sending
`SIGHUP` to `os.Getpid()` rather than holding a reloader of its own. We decided
that this is the boundary: a trigger asks for a reload and learns nothing about
what happened, so no trigger can grow its own idea of what reloading means.

The alternative had already failed once. Before the reloader existed, the
fsnotify and `SIGHUP` paths each applied a config by hand, and they disagreed —
fsnotify reported a failed hook reload while `SIGHUP` discarded the error and
logged success, leaving an invalid config's stale hooks in place with no signal
to the user.

## Considered options

- **Give the systray a real result path.** Rejected: the reloader is built
  inside `runCore`, on the far side of the goroutine that `Run` starts, so the
  systray component — constructed before it — cannot hold one. Reaching it means
  either lifting reloader construction up into `Run` or threading a result
  channel back down, and both restore a direct route from a UI surface into the
  reload path, which is the shape that drifted in the first place.
- **Route the file watcher through `SIGHUP` as well.** Rejected, though it is
  the tidiest version of this decision: it would leave exactly one goroutine
  that ever reloads, making single-threaded reload a structural fact instead of
  a convention, and would remove the need for any locking. It also collapses
  `reloadTrigger` to a single value — a file save and an external signal would
  log identically, and "did my editor do this, or did something else?" is a
  question worth being able to answer from the log.
- **Label a systray reload distinctly by setting a flag before signalling.**
  Rejected: an unrelated `SIGHUP` arriving between the flag and the signal would
  be labelled `systray`. Two honest labels beat three where one lies.

## Consequences

- **The systray reports a request, not a result.** Its menu item log line says
  the reload was requested, matching what `mimi config reload` already prints.
  Neither surface can honestly claim the config was applied, because neither
  waits to find out. The sanctioned way to close that gap is a last-reload
  status line in the systray menu, fed by the daemon's own reload outcome —
  a systray feature, designed on its own, not a second route into reloading.
- **`reloadTrigger` is deliberately coarse.** It has two values, `fsnotify` and
  `sighup`, and a systray click, a `mimi config reload`, and a hand-typed
  `kill -HUP` all arrive as the second one. That is not an omission to be
  filled in; the daemon genuinely cannot distinguish them at the signal
  boundary, and the options above are the reasons it should not pretend to.
- **Reload serialises itself.** Two goroutines can still apply a config — the
  watcher and the signal loop — so applying one holds a lock for its whole
  duration. Without it, a simultaneous file save and signal can interleave into
  a torn reload: hooks from one config, executor settings from another.
- **Restart-only settings stay restart-only, and say so.** Nothing in the reload
  path re-reads the logger, the pid file, the socket, the systray, or the hook
  worker count, and a reload that changes one of those fields reports that a
  restart is required rather than silently ignoring it. The set is derived from
  what applying a config actually reads, because the hand-maintained list of it
  was wrong within a day of being written.
