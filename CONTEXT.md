# mimi

mimi drives macOS windows and Mission Control spaces, either as a one-shot CLI
invocation or as a long-running daemon that fires shell hooks on desktop
events. This file is the project's glossary — the words we use for the domain,
and the words we have decided not to use.

## Language

**Action**:
One of the user-facing operations mimi can perform on the desktop:
`focus_window`, `space`, `move_window_to_space`, `resize_window`.
_Avoid_: command (that is one instance of an action), subcommand, verb

**Command**:
One fully-specified, validated instance of an action — the action's name
together with the arguments that action takes.
_Avoid_: request, invocation, job

**Request**:
The envelope one command travels in over the daemon's Unix socket. Reserved
for the wire; a command that never leaves the process is never a request.
_Avoid_: message, payload, packet

**Direct path**:
Running a command in the CLI's own process, with no daemon involved. What
`mimi action …` falls back to when no daemon is listening on `socket_file`.
_Avoid_: local execution, inline execution, fallback

**Daemon path**:
Running a command by sending it over the socket to the daemon, which performs
it on its own thread and returns the result.
_Avoid_: IPC path, remote execution, server path

**Space**:
One Mission Control space, identified by its 1-based index in Mission Control
ordering across every connected display.
_Avoid_: desktop, workspace, virtual desktop

**Hook**:
A shell command the daemon runs when a desktop event of a given kind occurs.
_Avoid_: handler, callback, listener, and _trigger_ — which names a reload
route (see **Reload trigger**), never a hook.

**Reload**:
Applying a new config to a running daemon without restarting it. A reload is
all-or-nothing: an invalid config leaves the previous, working one entirely in
place.
_Avoid_: refresh, re-init, restart (a restart is the other thing entirely)

**Reload trigger**:
One of the routes that asks a running daemon to reload: the config file
watcher, `SIGHUP`, the systray's reload menu item, and `mimi config reload`.
Every trigger produces the same outcome for the same config — a trigger
chooses _when_ a reload happens, never _what_ it does.
_Avoid_: reload source, reload event, reload signal

**Restart-only setting**:
A config field the running daemon reads once at startup and never re-reads, so
changing it takes effect only after the daemon is restarted. The complement of
the fields a reload applies.
_Avoid_: static setting, boot setting, startup setting

**Event kind**:
The category of desktop change a hook subscribes to — window focused, space
changed, app launched, and so on. One table defines the set; every other list
of kinds is derived from that table rather than restated alongside it.
_Avoid_: event type, event name
