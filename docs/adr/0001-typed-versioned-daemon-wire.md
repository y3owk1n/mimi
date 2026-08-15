# The daemon socket carries a typed, versioned command

**Status:** accepted

`mimi action` reaches the daemon over a Unix socket whose request was
`{action: string, args: []string}` — the CLI flattened its already-typed flags
into strings and the daemon re-parsed them with a hand-written flag parser.
Once the CLI began building a typed `action.Command` directly, that parser's
only remaining job was to undo a serialisation mimi itself performed twenty
lines earlier, and it left two independent implementations of every argument
rule to keep in agreement. We decided the socket carries the typed command as
JSON, alongside an integer protocol version, and deleted the string parser.

## Considered options

- **Keep the string wire and test the round trip.** Rejected: it makes the
  duplicate rule implementations permanent. They had already drifted — the same
  bad width reported `invalid width: -5` on one path and `invalid width: "-5"`
  on the other, and the two string parsers disagreed on whether a stray
  positional argument was an error.
- **Promote the string wire to a documented protocol.** Rejected: nothing
  outside mimi speaks it, and no document has ever described its payload. Making
  it public would have been a new product commitment, not a refactor.
- **Send `geometry.Request` over the wire.** Rejected: `geometry.Dimension`
  holds unexported fields, so `encoding/json` renders it as `{}` and reports no
  error. Every width and height would have silently vanished on the daemon path
  while working correctly on the direct path. The wire carries
  `action.ResizeWindowArgs` — the raw flags plus their `Set` bits — and the
  daemon converts after decoding.

## Consequences

- **Version skew is detected, not tolerated.** The daemon compares an integer
  protocol version for strict equality; a missing field decodes to zero and
  fails the same check, which is exactly the right answer for a CLI built before
  this change. `Version` was unusable for this — it is `"dev"` in every local
  build.
- **A mismatched daemon does not break the CLI.** `mimi action` falls back to
  direct execution, as it already does when no daemon is listening, and prints a
  warning naming the fallback and the fix. Hard-erroring would break every
  hotkey after an upgrade-without-restart, for a condition where the direct path
  works. Silent fallback was rejected because the skew would then persist
  forever — the same trap `docs/TROUBLESHOOTING.md` documents for a mismatched
  `socket_file`.
- **Argument rules are validated on both ends by one function.** The conversion
  from raw arguments to a geometry request *is* the validation, called once at
  the CLI for fail-fast and again after decode because the socket is a trust
  boundary. It is deliberately not a separate `Validate` method; a second
  implementation of the rules is the defect this ADR exists to remove.
- **`geometry.Request` now contradicts the glossary, deliberately.**
  `CONTEXT.md` reserves *request* for the wire envelope. Renaming the geometry
  type would churn files that had just settled, so the symbol stays. This is not
  an oversight to be tidied up.
- **An unknown resize preset must now be rejected by the type or the
  conversion.** `geometry.Request` documented that it ignores an unknown preset
  name because a parser stood in front of it. That parser is gone, so the
  guarantee moved into the value that crosses the wire.
