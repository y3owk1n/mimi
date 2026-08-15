# A setting the daemon never reads is reinstall-only

**Status:** accepted

`settings.service_path` sets the `PATH` the launchd-installed daemon runs with,
and therefore the `PATH` its hooks inherit. Nothing in the daemon reads it: it
is consumed by `mimi services install` when it renders the plist, and the value
that matters is the one baked into the plist on disk. `reloadability.go` admits
only `reloadable` and `restart-only` and rejects any field carrying neither, so
the field must be classified — and neither existing value is true. We decided to
add a third, `reinstall-only`, for fields whose new value takes effect when the
service is installed again, not when the daemon next starts.

The `reload:` tag was previously read as *when does the daemon pick this field
up* — at every reload, or once at startup. A field the daemon never reads at all
has no answer to that question. The tag is better understood as *what must the
user do for a change to this field to take effect*, which admits actions the
daemon plays no part in.

## Considered options

- **Tag it `restart-only`.** Rejected, and it is the option that would have been
  actively worse than not shipping the setting. ADR-0002 committed the daemon to
  reporting restart-only changes a reload could not apply, so this would make it
  advise a restart for a field a restart does not change — a specific, confident,
  wrong instruction, produced by machinery built to stop exactly that.
- **Put it in a new `[service]` section, tagged at section level.** Rejected as
  premature, not as wrong. `KeepAlive`, `Nice` and `ThrottleInterval` are all
  plausible future install-time settings, and the second one that appears is
  when this becomes the right shape. A section for one string is not.
- **Make it a `--path` flag on `services install`, with no config field.** This
  sidesteps the classification problem entirely, and was rejected because
  `install` is idempotent and re-renders the plist when the rendered bytes
  differ. A flag is not remembered, so the next plain `install` would silently
  revert the `PATH` — reintroducing the drift between what the config says and
  what the installed service does that placing the captured log streams beside
  `log_file` was meant to end.
- **Merge the whole `EnvironmentVariables` dict from config.** Rejected: the
  plist also carries the captured-stream paths the daemon reads back, so a
  user-supplied dict could overwrite keys the install path depends on. A single
  `PATH` string cannot collide with anything.

## Consequences

- **A fourth reload outcome.** `ReloadOutcome` gains `ReinstallRequired`
  alongside `Applied`, `RestartRequired` and `Failed`, with a daemon message
  naming `mimi services install`. Folding it into `RestartRequired` and
  accepting the wrong verb was considered and rejected for the same reason as
  tagging the field `restart-only`.
- **Restart-only is no longer the complement of reloadable.** ADR-0002 treats
  the restart-only set as everything a reload does not apply. That is now two
  sets, and the glossary in `CONTEXT.md` has been corrected to match.
- **Nothing implements this yet.** The decision comes out of an architecture
  review; the field, the tag value and the fourth outcome all ship together in
  the PR that adds `settings.service_path`. A reader grepping for
  `reinstall-only` before then will find only this file.
