# Troubleshooting

## `mimi action space` or `move_window_to_space` does nothing

1. **Rebuild** after updates — native run-loop pumping is required for CLI actions
2. **Grant Accessibility** to the exact binary you run (`bin/mimi` or `Mimi.app`)
3. **Check space index** — spaces are 1-based in Mission Control order (`mimi action space 1` is the first space)
4. **Close Mission Control** — actions refuse to run while Mission Control is open

## `mimi action` runs but seems to ignore the running daemon

`mimi action …` routes over the daemon's Unix socket (`settings.socket_file`)
when something is listening there, and otherwise falls back to running the
action directly, in the CLI's own process — see
[Configuration: socket_file](CONFIGURATION.md#socket_file). Both paths run
the same action and produce the same result, so this normally isn't visible.
It matters when:

- **The daemon and the CLI disagree on `socket_file`.** `mimi action` always
  resolves its own config path (the default when no `-c`/`--config` is
  given) and reads `socket_file` from *that* file. If the daemon was started
  against a different config, or `socket_file` was edited but the daemon not
  restarted (it's restart-only — see [Reloading](CONFIGURATION.md#reloading)),
  the CLI is checking a socket the daemon isn't on, actions fall back to
  direct execution silently, and nothing errors.
- **You're timing something.** The daemon path is a socket round trip to an
  already-running process; the direct path pays process startup cost instead
  but skips the socket. An action that behaves differently under load, or
  under a hotkey runner that expects the daemon's turnaround, may be taking
  the path you didn't expect.

To check which path an action actually took, run `mimi status` to confirm
the daemon is running and reachable, and compare its socket path against
`mimi config dump`'s `socketFile` for the same `-c`/`--config` your action
command uses.

## Window hooks not firing

1. Run `mimi status` — confirm daemon is running and Accessibility is granted
2. Run `mimi config validate` — confirm hooks are defined
3. Set `log_level = "debug"` in config and check logs
4. Window hooks require Accessibility; workspace hooks do not

## Daemon won't start

```bash
mimi config validate
mimi status          # check for stale PID file
rm ~/.local/share/mimi/mimi.pid
mimi start
```

## launchd service issues

```bash
mimi services status
launchctl list | grep mimi
cat /tmp/mimi.err.log    # if using Nix module
```

## Permission prompt keeps appearing

Remove and re-add mimi in System Settings → Privacy & Security → Accessibility. Ensure you're granting the binary you actually execute (Homebrew cask path vs local `bin/mimi`).
