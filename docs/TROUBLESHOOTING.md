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
cat /tmp/mimi.err.log    # Nix module, or mimi services install with log_file unset
```

### Where a service-installed daemon's console output lands

`mimi services install` writes a launchd plist that captures the daemon's
stdout and stderr to two files. They are **not** the structured log — that is
`settings.log_file`, written independently by mimi itself. The captured
streams are the plain console output, and they are the only place a crash
early in startup, before the logger exists, can surface.

`mimi services install` places them beside `settings.log_file`, in the same
directory and under the same name, with distinct suffixes. `~` is expanded
before it reaches the plist, which always stores absolute paths:

| `settings.log_file`            | stdout                             | stderr                             |
| ------------------------------ | ---------------------------------- | ---------------------------------- |
| `~/.local/state/mimi/mimi.log` | `~/.local/state/mimi/mimi.out.log` | `~/.local/state/mimi/mimi.err.log` |
| `/var/log/mimi/daemon.jsonl`   | `/var/log/mimi/daemon.out.log`     | `/var/log/mimi/daemon.err.log`     |
| unset, or not an absolute path | `/tmp/mimi.log`                    | `/tmp/mimi.err.log`                |

Notes on this:

- They never point at `settings.log_file` itself. That file belongs to the
  rotating file-log writer; a second process appending raw console output to
  it would corrupt the rotation.
- `settings.log_file` is optional. With it unset, there is no directory to
  derive from, so the streams keep the `/tmp/mimi.log` and `/tmp/mimi.err.log`
  the plist has always used — the same pair the Nix modules install. A
  `log_file` that is not an absolute path — a relative path, or a `~` that
  could not be expanded because the home directory was unresolvable — falls
  back the same way, since launchd expands neither and runs the job from `/`.
- **The captured streams are not rotated.** `settings.log_file` gets size,
  age, and backup limits; these two get none, while the same plist sets
  `KeepAlive = true`, so the daemon is restarted for as long as you are
  logged in. At `log_level = "debug"` mimi emits a console line per window
  event, so these files grow without bound. Truncate them yourself if they
  get large:

  ```bash
  : > ~/.local/state/mimi/mimi.out.log
  : > ~/.local/state/mimi/mimi.err.log
  ```

- launchd opens both files when it spawns the daemon and creates no
  directories of its own, so `mimi services install` creates
  `settings.log_file`'s directory for it. Install fails with a
  `creating log directory` error if that is not possible — fix `log_file` and
  install again.
- The paths are baked into the plist at install time. After changing
  `settings.log_file`, run `mimi services uninstall && mimi services install`
  to regenerate it — a restart alone keeps the old paths.
- The Nix modules (`nix/darwin.nix`, `nix/home.nix`) write their own plists,
  which always use `/tmp/mimi.log` and `/tmp/mimi.err.log` regardless of
  `settings.log_file`.

## Permission prompt keeps appearing

Remove and re-add mimi in System Settings → Privacy & Security → Accessibility. Ensure you're granting the binary you actually execute (Homebrew cask path vs local `bin/mimi`).
