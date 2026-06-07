# Security Policy

## Supported Versions

Only the **latest release** receives security fixes.

| Version        | Supported |
| -------------- | --------- |
| Latest release | Yes       |
| Older releases | No        |

---

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Report privately via [GitHub Security Advisories](https://github.com/y3owk1n/mimi/security/advisories/new) or contact [@y3owk1n](https://github.com/y3owk1n).

---

## Security Model

### Permissions

mimi requires **macOS Accessibility permission** for:

- `mimi action` commands (window focus, space switching, move window)
- Window hooks (`on_window_*`)

With Accessibility granted, mimi can read window metadata and synthesize input events for space switching. It does not record, transmit, or log UI content beyond what hooks need.

Workspace hooks (`on_workspace_changed`) do not require Accessibility.

### No Network Access

mimi makes no outbound network connections, sends no telemetry, and does not phone home.

### CGo / Objective-C

Native code lives in `internal/native/`. Space and window-to-space features use undocumented SkyLight private APIs. Report memory-safety issues in this layer promptly.

### Hook Execution

Hooks run shell commands with the daemon's user privileges. Do not put untrusted content into hook commands or config files.

### Private APIs

`mimi action space` and `mimi action move_window_to_space` use reverse-engineered private macOS APIs. They may break on OS updates and are not security-reviewed by Apple.
