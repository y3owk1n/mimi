# The tiling engine's state is read over the socket, and is not a query

**Status:** accepted

`mimi tiling state` prints what the daemon's tiling engine is remembering for
each display and space, and `mimi tiling reset` makes it forget. Both are
reads or writes of the daemon's own memory rather than of the desktop, so
neither can run the way the other reads do. We decided they travel the Unix
socket as the `tiling` action already does, and that they are not queries.

`CONTEXT.md` defines a **Query** as a read that "always runs on the direct
path and never travels the socket", and `docs/ARCHITECTURE.md` gives the
reason: a query has no side effect to serialize with the daemon's actions, so
routing it over the socket would cost a wire change and buy nothing. That
reasoning does not reach this state, because this state exists only inside the
daemon.

## Considered options

- **`mimi query tiling`, on the direct path like every other query.**
  Rejected: there is nothing to read. A CLI process with no daemon builds a
  throwaway engine whose state map is empty, so the command would print an
  empty answer that looks exactly like a daemon holding nothing. The two
  cases are the ones a user most needs told apart.
- **`mimi query tiling`, but routed over the socket.** Rejected: it would
  make "query" mean two different things depending on which query it was, and
  the definition that says otherwise is load-bearing. Someone reading
  `internal/action/query.go` should not have to check each function for
  whether it is the exception.
- **A subcommand of `tiling`, over the socket (chosen).** The `tiling`
  action already travels the socket and already reaches the engine through
  `ipc.Server.HandleDirect`, because the engine holds the layout's state and
  queues its own desktop work. A read of that same state belongs on the same
  path, under the same command family, and needs no new concept.

## Consequences

- **The response envelope carries data now.** `ipc.Response` gained a `data`
  field, and a direct handler answers with `(json.RawMessage, error)` rather
  than an error alone. This is additive, so `ipc.ProtocolVersion` does not
  move: an older daemon ignores a field it does not know, and an older CLI
  ignores one it is not looking for. Every action that drives the desktop
  still answers with success or failure and nothing else.
- **There is no fall back to a local engine.** `runTiling` falls back to an
  engine of the CLI's own when no daemon answers, because running the layout
  once is still useful. `printTiling` does not, because an engine built here
  has never run a pass and holds nothing. With no daemon it prints an empty
  answer and says on stderr that there was nothing to ask, which is the
  distinction the direct path could not make.
- **`mimi tiling preview` is still the one that works either way.** It runs
  the layout rather than reading the engine, so it needs no daemon. It also
  cannot answer the question `state` answers: a preview always runs with a
  null state, so it shows what a fresh space would do and never what the
  daemon is actually holding.
- **A space is named twice in the output.** The engine files state under the
  window server's identifier for a space, which never changes, and the user
  counts spaces by their place in Mission Control, which does. Reporting only
  the identifier would be unreadable and reporting only the place would be
  ambiguous, so both are printed, and the place is 0 for a space that is not
  in front on any display.
