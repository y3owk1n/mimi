# Troubleshooting

Start with `mimi doctor`. It runs the checks below that a program can run
and prints the fix under any that fails.

## `mimi action space` or `move_window_to_space` does nothing

mimi reads the space in front back after the swipe. When the destination
has not come in front within a second the action fails with `space N did
not come in front`, so a swipe macOS dropped is an error and not a silent
no-op. Then:

1. Rebuild after pulling changes, if you run a source build.
2. Grant Accessibility to the exact binary you run (`bin/mimi` or `Mimi.app`).
3. Check the space index. Spaces are 1-based in Mission Control order (`mimi action space 1` is the first space).
4. Close Mission Control. Both actions refuse to run while Mission Control is open.

### Space switching on macOS 27 and later

macOS 27 changed how the Dock reads a synthetic swipe. The gesture fields on the
posted `CGEvent` are no longer enough, and the event must also carry a
serialized IOHID payload. mimi picks the encoding from the running OS version,
so you do not need to configure anything.

If space switching misbehaves near that boundary, override the choice with
`MIMI_FORCE_DOCK_SWIPE_AUGMENTATION`. `1` forces the macOS 27 encoding, and any
other value forces the pre-27 one:

```bash
MIMI_FORCE_DOCK_SWIPE_AUGMENTATION=1 mimi action space next
```

mimi reads the variable once per process, so restart a running daemon for a
change to take effect. Failures to build the payload are logged with a
`Mimi: dock swipe augmentation failed` prefix.

## `mimi action` runs but seems to ignore the running daemon

`mimi action ...` sends the action over the daemon's Unix socket
(`settings.socket_file`) when something is listening there. Otherwise it runs
the action directly, in the CLI's own process. See
[Configuration: socket_file](CONFIGURATION.md#socket_file). Both paths run the
same action and produce the same result, so you do not normally see the
difference. It matters in these cases:

- The daemon and the CLI disagree on `socket_file`. `mimi action` resolves its
  own config path (the default search order when no `-c`/`--config` is given)
  and reads `socket_file` from that file. The daemon may have started with a
  different config, or `socket_file` may have changed without a daemon
  restart. The setting is restart-only, see [Reloading](CONFIGURATION.md#reloading).
  In either case the CLI checks a socket the daemon is not on, actions fall
  back to direct execution, and nothing reports an error.
- mimi was upgraded and the daemon was not restarted. The daemon path sends a
  versioned request, and the daemon accepts only the version its own build
  speaks. A daemon running a different build rejects the request, and
  `mimi action` runs the command on the direct path instead. It prints one
  line to stderr that names the mismatch and the fix. The action still does
  what you asked and exits as before, so this is a warning, not a failure.
  The version check rejects skew in both directions. A daemon newer than the
  `mimi` binary on your `PATH` refuses that binary's requests too. A daemon
  older than the version check cannot recognise a request it cannot read, and
  may fail the action with an error about its arguments. The fix is the same
  in every case. Restart the daemon so it runs the same build as the CLI, with
  `mimi stop && mimi start`, or `mimi services restart` when it runs as a
  launchd service.
- You are timing something. The daemon path is a socket round trip to a
  process that is already running. The direct path skips the socket but pays
  the cost of starting a process. If an action behaves differently under load,
  or under a hotkey runner that expects the daemon's response time, it may be
  taking the path you did not expect.

To check which path an action will take, run `mimi status` with the same
`-c`/`--config` as the action. It loads the same config. `ipc: socket available
at <path>` means a socket file exists where the action looks, and
`ipc: socket not available` means actions run directly. To see where a daemon
listens, run `mimi config dump` against the config the daemon started with and
read `socketFile`.

## Window hooks not firing

1. Run `mimi status` to confirm the daemon is running and Accessibility is granted.
2. Run `mimi config validate` to confirm the config parses and the hooks are defined.
3. Run `mimi hooks fire <kind> --app <name>` with the values the real event
   would carry. It reports every hook of the kind as matched or skipped with
   the reason, and shows what a matched one printed, without the daemon.
4. Set `log_level = "debug"` in the config and check the logs.
5. Window hooks require Accessibility. Workspace hooks do not.

## A hook works by hand, but does nothing under the installed service

The installed service does not inherit your login shell's `PATH`. launchd
gives it the `PATH` written in the plist, which is
`/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin` unless
[`settings.service_path`](CONFIGURATION.md#service_path) sets another. A hook
that calls anything in `~/.local/bin`, a Nix profile, or a language version
manager therefore works from a terminal and fails under the service. The
daemon's captured stderr usually shows "command not found".

Set the whole `PATH` you need and install again. The setting is
reinstall-only, so only `mimi services install` applies it:

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

`mimi start` overwrites a stale PID file. Until then, `mimi status` reports
`not running (stale PID file)`. `mimi start` runs in the
foreground, so it prints any startup error to the terminal.

## launchd service issues

```bash
mimi services status
launchctl list | grep mimi
cat /tmp/mimi.err.log    # Nix module, or mimi services install with log_file unset
```

### Reading `mimi services status`

Loaded is not the same as running. The installed plist sets `KeepAlive` with a
ten second `ThrottleInterval`, so launchd relaunches a daemon that crashes at
startup indefinitely, and the service stays loaded the whole time.

| Line                                                  | What it means                                                                                                              |
| ----------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| `Service loaded and running (pid 1478)`               | Healthy. launchd has a live process for it.                                                                                 |
| `Service loaded but not running (last exit status 1)` | launchd is respawning it. A non-zero status that does not change is a crash loop. The captured stderr below says why.       |
| `Service loaded`                                      | Neither number was available. The daemon has never run, a signal killed it rather than it exiting, or mimi could not read launchd's description of the job. That output is undocumented, so when mimi cannot parse it the line loses the detail but still reports that the service is loaded. |
| `Service not loaded`                                  | No service is installed, or it was unloaded.                                                                                |
| `Service state unknown: launchctl could not be run`   | `launchctl` could not run at all, because it is missing from `PATH` or could not be spawned. mimi learned nothing about the service, which may still be running. `mimi services install` refuses to run in this state, and `uninstall` fails instead of ignoring a failed unload. |

`launchctl print gui/$(id -u)/com.y3owk1n.mimi` prints the full description mimi
reads, for when a bare `Service loaded` leaves a question open.

Below that line, the status lists the captured console streams that the
installed plist names, with the size of each:

```text
Service loaded and running (pid 1478)
Captured stdout: /Users/me/.local/state/mimi/mimi.out.log (2.0 KB)
Captured stderr: /Users/me/.local/state/mimi/mimi.err.log (not created yet)
```

The size covers one run's console output, because a daemon that launchd started
empties both files at startup (see below). `not created yet` means launchd has
never spawned the daemon with that file. Neither line appears when there is no
plist to read the paths from. That happens when nothing is installed, or when
`~/Library/LaunchAgents/com.y3owk1n.mimi.plist` is not a regular file mimi can
read, such as a symlink.

### Where a service-installed daemon's console output lands

`mimi services install` writes a launchd plist that captures the daemon's
stdout and stderr to two files. They are not the structured log, which is
`settings.log_file` and which mimi writes itself. The captured streams hold the
plain console output. A crash early in startup, before the logger exists, shows
up only there.

`mimi services install` places them next to `settings.log_file`, in the same
directory, with the same name and different suffixes. mimi expands `~` before
writing the plist, so the plist always stores absolute paths:

| `settings.log_file`            | stdout                             | stderr                             |
| ------------------------------ | ---------------------------------- | ---------------------------------- |
| `~/.local/state/mimi/mimi.log` | `~/.local/state/mimi/mimi.out.log` | `~/.local/state/mimi/mimi.err.log` |
| `/var/log/mimi/daemon.jsonl`   | `/var/log/mimi/daemon.out.log`     | `/var/log/mimi/daemon.err.log`     |
| unset, or not an absolute path | `/tmp/mimi.log`                    | `/tmp/mimi.err.log`                |

Notes:

- The streams never point at `settings.log_file` itself. The rotating file-log
  writer owns that file, and a second process that appends raw console output
  to it would break the rotation.
- `settings.log_file` is optional. When it is unset there is no directory to
  derive from, so the streams use `/tmp/mimi.log` and `/tmp/mimi.err.log`, the
  same pair the Nix modules use. A `log_file` that is not an absolute path
  falls back the same way. That covers a relative path, or a `~` that mimi
  could not expand because it could not find the home directory. launchd
  expands neither and runs the job from `/`.
- The captured streams are limited to one run, not rotated.
  `settings.log_file` gets size, age, and backup limits from the writer that
  owns it. These two files get none, and nothing can rotate them. launchd opens
  both at spawn and holds the descriptors, and the same plist sets
  `KeepAlive = true`, so launchd restarts the daemon for as long as you are
  logged in. At `log_level = "debug"` mimi writes a console line per window
  event, and a crash loop respawns every ten seconds, so without a limit the
  files would grow without bound.

  Instead, a daemon that launchd started empties both files once at startup,
  before it writes anything to them. Each file therefore holds the console
  output of the current run and nothing older, and `mimi services status`
  prints the size of each.

  The plist tells the daemon which files these are, in its
  `MIMI_CAPTURED_STDOUT` and `MIMI_CAPTURED_STDERR` environment entries. A
  daemon started any other way, such as `mimi start` by hand with a terminal on
  stdout, sees neither entry and empties nothing. Running mimi in a shell
  therefore never deletes the crash log the service left behind. The next
  service start does, so copy a run's output elsewhere before you restart the
  service. You can also truncate the files by hand at any time:

  ```bash
  cp ~/.local/state/mimi/mimi.err.log /tmp/mimi-crash.log   # keep it past the restart
  : > ~/.local/state/mimi/mimi.out.log
  : > ~/.local/state/mimi/mimi.err.log
  ```

  A service installed before this behaviour existed keeps appending until you
  run `mimi services install` again, which adds the two environment entries to
  its plist.

- launchd opens both files when it spawns the daemon and does not create
  directories, so `mimi services install` creates the directory of
  `settings.log_file`. If it cannot, install fails with a
  `creating log directory` error. Fix `log_file` and install again.
- The paths are written into the plist at install time. After you change
  `settings.log_file`, run `mimi services install` again to regenerate it. A
  restart alone keeps the old paths. Running install again is safe. It replaces
  the plist and reloads the service when the rendered plist differs, and prints
  `Service already up to date` and changes nothing when it does not.
- The Nix modules (`nix/darwin.nix`, `nix/home.nix`) write their own plists,
  which always use `/tmp/mimi.log` and `/tmp/mimi.err.log` whatever
  `settings.log_file` says. Both set the two `MIMI_CAPTURED_*` entries from the
  paths their own job writes to, so a daemon started by a module empties those
  files at startup in the same way. A service from a module version before
  this behaviour existed keeps appending until the next `darwin-rebuild switch`
  or `home-manager switch` rewrites the plist. If you override the job's
  `StandardOutPath` or `StandardErrorPath`, the matching environment entry
  follows it. Setting either entry through `services.mimi.extraEnvironment` has
  no effect, because an entry that names a file the job does not write to
  would empty the wrong log.

## Permission prompt keeps appearing

Remove and re-add mimi in System Settings > Privacy & Security > Accessibility. Grant the binary you actually run (the Homebrew cask's `Mimi.app` or a local `bin/mimi`).
