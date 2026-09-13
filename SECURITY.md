# Security policy

## Supported versions

Only the latest release receives security fixes.

| Version        | Supported |
| -------------- | --------- |
| Latest release | Yes       |
| Older releases | No        |

---

## Reporting a vulnerability

Do not open a public GitHub issue for a security vulnerability.

Report it privately through [GitHub Security Advisories](https://github.com/y3owk1n/mimi/security/advisories/new) or contact [@y3owk1n](https://github.com/y3owk1n).

---

## Security model

### Permissions

mimi needs macOS Accessibility permission for:

- `mimi action` commands (window focus, space switching, moving and resizing windows)
- Some `mimi query` commands (each command's help says whether it needs the grant)
- Window hooks (`on_window_*`)
- Tiling and window borders in the daemon

With Accessibility granted, mimi reads window metadata and synthesizes input events for space switching. It does not record, transmit, or log UI content beyond what hooks need.

Workspace hooks (`on_workspace_changed`) and app hooks (`on_app_*`) do not need Accessibility.

### No network access

mimi makes no outbound network connections and sends no telemetry.

### CGO and Objective-C

Native code lives in `internal/native/`, `internal/systray/`, and `internal/permissions/`. Space and window-to-space features use undocumented SkyLight private APIs. Report memory-safety issues in this layer promptly.

### Hook execution

Hooks run shell commands with the daemon's user privileges. Do not put untrusted content into hook commands or config files.

### Private APIs

`mimi action space` and `mimi action move_window_to_space` use reverse-engineered private macOS APIs. They may break on OS updates, and Apple has not reviewed them for security.
