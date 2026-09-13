---
name: ask-mimi
description: "Answer a mimi user's question about what mimi does, which command or config key does a thing, or what to do next, from the man pages and help on their install rather than from memory. Routes setup work to setup-config and setup-layout. Use when a mimi user asks what mimi can do, how to do something with it, which command to run, or which skill to use."
---

# Answering questions about mimi

mimi is a macOS window and space tool. It has a CLI for immediate actions,
a daemon that runs shell hooks on app, window, and space events, and a
tiling engine that runs a layout program the user chooses. Every answer
about it should come from the installed version, since commands and keys
change between releases.

## What every install has

Check these before anything remote. Homebrew, Nix, and a source build all
ship them.

- `mimi --help`, then `mimi <command> --help`. The help text lists the
  flags and accepted values of the installed version.
- `man mimi`, and one page per subcommand such as `man mimi-action-resize_window`
  and `man mimi-tiling-cmd`. `apropos mimi` lists them.
- `mimi status` for whether the daemon runs and Accessibility is granted.
- `mimi config dump` for the config in force, defaults filled in.
- `mimi query space`, `window`, `windows`, `displays`, and `margins` for
  the current spaces, windows, and displays as JSON.

For anything the help does not cover, fetch the doc at the installed
version:

```bash
tag=$(mimi --version | sed -n '1s/^Mimi version //p')
case $tag in v*) ;; *) tag=main ;; esac
curl -fsSL "https://raw.githubusercontent.com/y3owk1n/mimi/$tag/docs/CLI.md"
```

The docs are `CLI.md` for every command and flag, `CONFIGURATION.md` for
every key and hook, `TILING.md` for layouts, `TROUBLESHOOTING.md` when
something does not work, and `INSTALLATION.md` for install methods and the
launchd service.

## What mimi does

| Ask | Answer with |
| --- | --- |
| Switch space, move a window to a space or display | `mimi action space`, `move_window_to_space`, `move_window_to_display` |
| Focus a window by direction, app, or cycle | `mimi action focus_window`, `focus_app` |
| Resize or place a window by preset or size | `mimi action resize_window` |
| Apply frames from any program | `mimi action apply_frames` |
| Read the desktop as JSON | `mimi query ...` |
| Run a command when an app, window, or space changes | `[hooks]` in config, see `setup-config` |
| Draw a border around the focused window | `[border]` in config, see `setup-config` |
| Tile windows | `[tiling]` plus a layout program, see `setup-layout` |
| Show the active space number in the menu bar | `[systray]` in config, see `setup-config` |
| Run mimi at login | `mimi services install`, see `setup-config` |

Three things a user often does not know:

- **The daemon is optional.** Every `mimi action` works from the CLI
  alone. The daemon adds hooks, borders, tiling, and the menu bar item
  with the active space number. While it runs, the CLI sends actions over
  its socket, which is faster than starting each one from scratch.
- **mimi ships no layout.** Tiling means naming a program in config. The
  repo has six to copy, and `setup-layout` fetches them without a checkout.
- **Accessibility is the only permission mimi asks for.** Actions,
  window hooks, borders, and tiling all need it. `mimi status` says
  whether it is granted.

## Routing

- Config, hooks, borders, systray, or service work goes to `setup-config`.
- Tiling, layouts, and hotkeys for layout commands go to `setup-layout`.
- A question with a one-command answer gets the command and the help page
  that documents it.
- When something does not work, run `mimi status`, then fetch the
  Troubleshooting doc as above, before guessing at a cause.
- A bug or a missing feature goes to a GitHub issue on `y3owk1n/mimi`.
  Blank issues are disabled, so use the issue forms.

Do not answer flags or key names from memory. Run `--help` or read the man
page first.
