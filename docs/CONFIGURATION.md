# Configuration

mimi reads a TOML config from `~/.config/mimi/config.toml` (or `$XDG_CONFIG_HOME/mimi/config.toml`).

```bash
mimi config init       # create default config
mimi config validate   # check for errors
mimi config dump       # print resolved config as JSON
mimi config reload     # reload running daemon (SIGHUP)
```

---

## Reloading

`mimi config reload` (SIGHUP) and the systray's reload menu item both trigger
the same reload, and the daemon also picks up an edit to the config file on
disk automatically (fsnotify). All three routes reload `[hooks]` and the
subset of `[settings]` listed below; a bad config (e.g. an invalid `title`
regex) leaves the previous, working config in place and logs the failure
rather than applying it partially.

**Reloadable** — picked up by every reload:

- `settings.hook_timeout_secs`
- `settings.hook_shell`
- `settings.resize_debounce_ms`
- `[hooks]` (every hook kind)

**Restart-only** — read once at daemon startup and never re-read, so changing
one takes effect only after the daemon is restarted (`mimi daemon stop &&
mimi daemon start`, or equivalent):

- `settings.log_file`
- `settings.log_level`
- `settings.log_format`
- `settings.max_hook_workers`
- `settings.pid_file`
- `settings.socket_file`
- `systray.enabled`
- `systray.show_workspace_number`

**Reinstall-only** — never read by the daemon at all. It is baked into the
launchd plist by `mimi services install`, so the value in effect is the one in
the installed plist, and only installing the service again replaces it. A
restart does not:

- `settings.service_path`

A reload that changes a restart-only or reinstall-only setting still applies
everything reloadable, and then logs a warning naming the settings it could not
apply and what each of them needs, for example:

```text
config reloaded; restart required for changed restart-only settings
  trigger=sighup restart_only=["settings.log_level","settings.max_hook_workers"]
```

```text
config reloaded; run `mimi services install` for changed reinstall-only settings
  trigger=sighup reinstall_only=["settings.service_path"]
```

A reload that changes both logs both lines, in that order.

A reload that changes only reloadable settings logs the plain
`config reloaded` line, with neither notice.

The comparison is against the config the daemon started with, not the previous
edit, so the notice keeps appearing on every reload for as long as the file and
the running daemon disagree — and stops on its own if you put the old value
back.

With `systray.enabled = true`, the menu shows the same outcome to someone who
has no log in front of them: a disabled line under **Reload Config** reading
`Reloaded 14:32`, `Reloaded 14:32 — restart required`,
`Reloaded 14:32 — run mimi services install`, or `Reload failed 14:32`, and
`No config reload yet` until the daemon has reloaded once. It reports the
daemon's own reload, so every route above updates it, not just the menu item.
The line shows one outcome, so a reload that needs both a restart and a
reinstall shows the restart — the instruction that holds however the daemon
was started — and the log names both.

These lists are not maintained by hand: each config field is classified on the
type itself, and a test fails if this document and that classification
disagree.

---

## Settings

```toml
[settings]
log_file = "~/.local/share/mimi/mimi.log"   # optional; omit for console-only — restart-only
log_level = "info"                           # debug | info | warn | error — restart-only
log_format = "text"                          # text | json — console output only — restart-only
hook_timeout_secs = 10
hook_shell = "/bin/sh"
max_hook_workers = 4                         # restart-only
pid_file = "~/.local/share/mimi/mimi.pid"    # restart-only
socket_file = "~/.local/share/mimi/mimi.sock" # restart-only
resize_debounce_ms = 250                     # on_window_resize debounce window
service_path = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin" # PATH for the installed service — reinstall-only
```

`log_format` selects the **console** encoder only: `text` is the human-readable
line (colorized when the console is a terminal), `json` is one JSON object per
line with no color. The `log_file` log is always JSON, whatever `log_format`
says, so anything already piping that file through `jq` keeps working. An
unrecognized value logs a warning and falls back to `text`.

### socket_file

`socket_file` is the Unix socket the daemon listens on and `mimi action …`
looks for it on. Every `mimi action` invocation resolves its config path
first (the default path when `-c`/`--config` is not given) and reads
`socket_file` from it, then checks that socket:

- **A daemon is listening** — the action is sent over the socket and runs on
  the daemon's dedicated OS thread.
- **Nothing is listening** — the CLI falls back to running the action
  directly, in the CLI's own process.

Both paths produce the same result, but not with the same timing: the daemon
path is a socket round trip; the direct path runs in-process with no daemon
involved at all. If you run the daemon with a non-default `socket_file`, that
value is what routes `mimi action` to it — a mismatch between the two (or a
daemon that hasn't picked up a changed `socket_file`, since it is
restart-only) means actions silently execute directly instead of reaching the
daemon. See [Troubleshooting](TROUBLESHOOTING.md#mimi-action-runs-but-seems-to-ignore-the-running-daemon)
for how to tell which path an action actually took.

### service_path

`service_path` is the `PATH` `mimi services install` writes into the launchd
plist, and therefore the `PATH` the installed daemon — and every hook it runs
— inherits. Unset, it is
`/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin`, which is what the plist has
always hardcoded.

Set it when a hook works from your shell but does nothing under the installed
service. A login shell's `PATH` never reaches a launchd agent, so a hook
calling something in `~/.local/bin`, a Nix profile, or a language version
manager needs that directory listed here:

```toml
[settings]
service_path = "/Users/me/.local/bin:/run/current-system/sw/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
```

It is the whole `PATH`, not an addition to one: what you write is what the
service gets, so keep the directories you still need. Write absolute
directories: unlike `log_file` and the other path settings, `~` is not expanded
here, and launchd does not expand it either — a `~/.local/bin` entry is a
directory the service will never find anything in.

It is reinstall-only. The daemon never reads it — the value that matters is the
one already in the installed plist — so run `mimi services install` after
changing it. That re-renders the plist and reloads the service; a `mimi
services restart`, or a plain daemon restart, keeps the old `PATH`. Running
`mimi start` by hand ignores this setting entirely: that daemon inherits the
`PATH` of whatever started it.

If your service comes from the nix-darwin or home-manager module rather than
from `mimi services install`, that module renders its own agent and this
setting does not reach it — set `services.mimi.extraEnvironment.PATH` there
instead.

### Debug logging

`log_level = "debug"` adds two extra lines per routed event: a router
`"event"` line (one per event reaching the bus) and, for each hook
registered on that event's kind, an executor `"hook matched"` or
`"hook skipped"` line.

Both record only counts, IDs, kinds, PIDs, and booleans: `kind`, `app`,
`bundle`, `pid`, `event_id`, the hook's `index` within its kind (0-based,
scoped to hooks of the same kind — enough to tell two hooks apart in the
log), the `reason` a hook was skipped (a fixed string, e.g.
`"app filter mismatch"`), and `title_present` (whether the window has a
non-empty title). They never record the window title itself or a hook's
`run` command text, even at `debug`.

The `"hook ok"` / `"hook timed out"` / `"hook failed"` lines emitted after a
hook finishes running follow the same rule: they identify the hook by the
same `index`, and none of them records the `run` command text.

One gap remains. The `"hook ok"` line still logs the hook's own captured
stdout and stderr as `output` (trimmed, and capped at 64 KiB). That is hook
output, i.e. a user payload, and it is an open exception to AGENTS.md's
"never log ... other user payloads" rule. It is emitted only at
`log_level = "debug"`; at `info` and above no hook output reaches the log,
and the `"hook failed"` line reports the failure through `exit` alone.

---

## Systray

```toml
[systray]
enabled = true                 # restart-only
show_workspace_number = true   # show active space number in menu bar — restart-only
```

---

## Hooks

The hook kinds below are the complete set — `[hooks]` accepts no other keys. A
key that is not one of them is a hook that can never fire, so `mimi config
validate` rejects it and the daemon logs a warning at startup and on reload
while running the hooks it did understand.

### Application Lifecycle

| Hook | Fires when |
| ---- | ---------- |
| `on_app_activate` | App comes to foreground |
| `on_app_deactivate` | App loses foreground |
| `on_app_launch` | App process starts |
| `on_app_quit` | App process terminates |
| `on_app_hide` | App hidden (⌘H) |
| `on_app_unhide` | Hidden app shown again |

### Window events (requires Accessibility)

| Hook | Fires when |
| ---- | ---------- |
| `on_window_focus` | Focused window changes |
| `on_window_title_change` | Active window title changes |
| `on_window_created` | New window opens |
| `on_window_closed` | Window closes |
| `on_window_resize` | Window resize completes (debounced) |

### Workspace events

| Hook | Fires when |
| ---- | ---------- |
| `on_workspace_changed` | Active Mission Control space changes |

### Hook entry format

```toml
[hooks]
on_window_focus = ["echo focus: $mimi_APP_NAME"]

on_window_focus = [
  { run = "notify-send focus", app = "Slack", async = true }
]
```

| Field | Description |
| ----- | ----------- |
| `run` | Shell command (required) |
| `app` | Filter by app name (glob) |
| `bundle_id` | Filter by bundle ID (exact) |
| `title` | Filter by window title (regex) |
| `timeout_secs` | Override global timeout |
| `async` | Run in background (default: false) |

---

## Environment Variables

Every hook receives:

| Variable | Description |
| -------- | ----------- |
| `mimi_EVENT` | Event kind (e.g. `app_activate`, `window_focus`, `workspace_changed`) |
| `mimi_EVENT_ID` | Unique event UUID |
| `mimi_APP_NAME` | App display name |
| `mimi_BUNDLE_ID` | Bundle identifier |
| `mimi_PID` | Process ID |
| `mimi_WINDOW_TITLE` | Window title (window events only) |
| `mimi_TIMESTAMP` | RFC3339 timestamp |
| `mimi_WINDOWS_COUNT` | Window count (workspace events only) |
| `mimi_INFO` | JSON workspace info (workspace events only) |

Use `$mimi_APP_NAME` or `${mimi_WINDOW_TITLE}` in hook commands. Each value is
substituted as a single, self-quoted shell token, so write the reference
**without** wrapping it in your own quotes:

```toml
# Correct — the value quotes itself:
on_window_title_change = [{ run = "notify-send $mimi_WINDOW_TITLE" }]
```

This matters because a value can contain anything — a window title is chosen by
whatever web page or document is open — and mimi runs the command through a
shell. Self-quoting stops a crafted title from breaking out and running as its
own command. One consequence: a reference placed inside your own double quotes
(`"... $mimi_WINDOW_TITLE ..."`) will show the wrapping quotes literally; leave
it unquoted instead.

---

## Example

```toml
[hooks]
on_app_activate = [
  { run = "echo active: $mimi_APP_NAME", async = true }
]

on_window_focus = [
  { run = "echo focus >> ~/window.log", app = "Code", async = true }
]

on_workspace_changed = [
  "echo 'switched space' >> ~/space.log"
]
```
