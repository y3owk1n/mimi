# Troubleshooting

## `mimi action space` or `move_window_to_space` does nothing

1. **Rebuild** after updates — native run-loop pumping is required for CLI actions
2. **Grant Accessibility** to the exact binary you run (`bin/mimi` or `Mimi.app`)
3. **Check space index** — spaces are 1-based in Mission Control order (`mimi action space 1` is the first space)
4. **Close Mission Control** — actions refuse to run while Mission Control is open

### Space switching on macOS 27 and later

macOS 27 changed how the Dock reads a synthetic swipe: the gesture fields on the
posted `CGEvent` are no longer enough on their own, and the event must also carry
a serialized IOHID payload. mimi picks the encoding from the running OS version,
so no configuration is needed.

If space switching misbehaves near that boundary, override the choice with
`MIMI_FORCE_DOCK_SWIPE_AUGMENTATION` — `1` forces the macOS 27 encoding, `0`
forces the pre-27 one:

```bash
MIMI_FORCE_DOCK_SWIPE_AUGMENTATION=1 mimi action space next
```

The variable is read once per process, so a daemon has to be restarted for a
change to take effect. Failures to build the payload are logged with a
`Mimi: dock swipe augmentation failed` prefix.

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
- **mimi was upgraded and the daemon not restarted.** The daemon path carries
  a versioned request, and the daemon accepts only the version its own build
  speaks. A daemon running a different build therefore rejects the request,
  and `mimi action` runs the command on the direct path instead, printing one
  line to stderr naming the mismatch and the fix. The action itself still does
  what it was asked and exits as it always did, so this is a warning, not a
  failure. Skew is rejected whichever build is ahead — a daemon newer than the
  `mimi` binary on your `PATH` refuses that binary's requests just the same.
  A daemon older than the version check itself has no way to recognise a
  request it cannot read, and may instead fail the action outright with a
  complaint about its arguments. Either way the fix is the same: restart the
  daemon so it runs the same build as the CLI — `mimi stop && mimi start`, or
  `mimi services restart` when it is installed as a launchd service.
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

## A hook works by hand, but does nothing under the installed service

The installed service does not inherit your login shell's `PATH` — launchd
gives it the one baked into the plist, which is
`/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin` unless
[`settings.service_path`](CONFIGURATION.md#service_path) says otherwise. A hook
calling anything in `~/.local/bin`, a Nix profile, or a language version
manager therefore runs from a terminal and fails from the service, usually
with the daemon's captured stderr showing a "command not found".

Set the whole `PATH` you need and install again — the setting is
reinstall-only, so nothing shorter of that applies it:

```toml
[settings]
service_path = "/Users/me/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
```

```bash
mimi services install
```

Or make the hook independent of `PATH` by calling absolute paths.

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

### Reading `mimi services status`

Loaded is not the same as running. The installed plist sets `KeepAlive` with a
ten second `ThrottleInterval`, so a daemon that crashes at startup is
relaunched forever and stays loaded the whole time.

| Line                                                  | What it means                                                                                                              |
| ----------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| `Service loaded and running (pid 1478)`               | Healthy: launchd has a live process for it.                                                                                 |
| `Service loaded but not running (last exit status 1)` | launchd is respawning it. A non-zero status that stays put is a crash loop — the captured stderr below says why.             |
| `Service loaded`                                      | Neither number was available: the daemon has never run, it was killed by a signal rather than exiting, or launchd's description of the job could not be read. That output is undocumented, so an unreadable one costs the detail, never the answer. |
| `Service not loaded`                                  | No service installed, or it was unloaded.                                                                                   |
| `Service state unknown: launchctl could not be run`   | `launchctl` could not be run at all: missing from `PATH`, or unable to be spawned. Nothing was learned about the service — it may well be running. `mimi services install` refuses to run at all in this state, and `uninstall` stops forgiving an unload that fails. |

`launchctl print gui/$(id -u)/com.y3owk1n.mimi` is the same description mimi
reads, in full, when a bare `Service loaded` leaves a question open.

Under that line come the captured console streams, as the installed plist
names them, with how large each has grown:

```text
Service loaded and running (pid 1478)
Captured stdout: /Users/me/.local/state/mimi/mimi.out.log (2.0 KB)
Captured stderr: /Users/me/.local/state/mimi/mimi.err.log (not created yet)
```

The size is one run's console output, because a daemon launchd started empties
both files at startup (see below). `not created yet` is a file launchd has
never spawned the daemon against. Neither line is printed when there is no
plist to read the paths from: nothing installed, or a plist mimi cannot read —
home-manager's, for one, is a symlink into the Nix store rather than a file.

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
- **The captured streams are bounded by one run, not rotated.**
  `settings.log_file` gets size, age, and backup limits from the writer that
  owns it; these two get none, and nothing can rotate them — launchd opens both
  at spawn and holds the descriptors, while the same plist sets
  `KeepAlive = true`, so the daemon is restarted for as long as you are logged
  in. At `log_level = "debug"` mimi emits a console line per window event, and
  a crash loop respawns every ten seconds, so they would otherwise grow without
  bound.

  Instead, a daemon launchd started empties both files once at startup, before
  it writes anything to them. Each file therefore holds the console output of
  the current run and nothing older, and `mimi services status` prints how
  large each has grown.

  The plist is what tells the daemon those files exist, in its
  `MIMI_CAPTURED_STDOUT` and `MIMI_CAPTURED_STDERR` environment entries. A
  daemon started any other way — `mimi start` by hand, with a terminal on
  stdout — is told about no captured streams and empties nothing, so running
  mimi in a shell never destroys the crash log the service left behind. The
  next start does, though, so copy a run's output aside before restarting the
  service — and truncate by hand whenever you want to:

  ```bash
  cp ~/.local/state/mimi/mimi.err.log /tmp/mimi-crash.log   # keep it past the restart
  : > ~/.local/state/mimi/mimi.out.log
  : > ~/.local/state/mimi/mimi.err.log
  ```

  A service installed before this behaviour existed keeps appending until
  `mimi services install` is run again — that is what puts the two environment
  entries into its plist.

- launchd opens both files when it spawns the daemon and creates no
  directories of its own, so `mimi services install` creates
  `settings.log_file`'s directory for it. Install fails with a
  `creating log directory` error if that is not possible — fix `log_file` and
  install again.
- The paths are baked into the plist at install time. After changing
  `settings.log_file`, run `mimi services install` again to regenerate it — a
  restart alone keeps the old paths. Install is idempotent: it replaces the
  plist and reloads the service when the rendered plist differs, prints
  `Service already up to date` and does nothing when it does not.
- The Nix modules (`nix/darwin.nix`, `nix/home.nix`) write their own plists,
  which always use `/tmp/mimi.log` and `/tmp/mimi.err.log` regardless of
  `settings.log_file`. Both carry the two `MIMI_CAPTURED_*` entries, filled
  from the paths their own job writes to, so a module-installed daemon empties
  those files at startup exactly as an installed one does. A service from a
  module version before this behaviour existed keeps appending until the next
  `darwin-rebuild switch` or `home-manager switch` — that is what rewrites the
  plist. Overriding the job's `StandardOutPath` or `StandardErrorPath` moves
  the matching environment entry with it; setting either entry through
  `services.mimi.extraEnvironment` has no effect, since one naming a file the
  job does not write to would empty the wrong log.

## Permission prompt keeps appearing

Remove and re-add mimi in System Settings → Privacy & Security → Accessibility. Ensure you're granting the binary you actually execute (Homebrew cask path vs local `bin/mimi`).
