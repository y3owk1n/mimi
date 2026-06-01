# Security Policy

## Supported Versions

Only the **latest release** receives security fixes. We do not back-port patches to older versions.

| Version        | Supported          |
| -------------- | ------------------ |
| Latest release | ✅ Yes             |
| Older releases | ❌ No              |

---

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Instead, report them privately:

1. **GitHub Security Advisories (preferred)** — go to [Security → Report a vulnerability](https://github.com/y3owk1n/mimi/security/advisories/new) on this repository. This creates a private advisory only visible to maintainers.
2. **Direct contact** — reach out to [@y3owk1n](https://github.com/y3owk1n) via GitHub if you cannot use the advisory flow.

Please include:

- A description of the vulnerability and its potential impact.
- Steps to reproduce or a proof-of-concept.
- The version(s) affected.
- Any suggested fix, if you have one.

### What to Expect

- **Acknowledgment** within **48 hours** of your report.
- A fix or mitigation plan within **7 days** for confirmed vulnerabilities.
- Credit in the release notes (unless you prefer to remain anonymous).

---

## Security Model

Understanding mimi's security posture helps frame what constitutes a vulnerability.

### Permissions

mimi requires **macOS Accessibility permission** for window-related events (focus, title change, etc.). Other event kinds (power, network, USB, display, audio, clipboard) use standard macOS notification APIs and do not require special permissions.

With Accessibility permission granted, mimi can:

- Read window metadata (titles, positions, ownership) via the Accessibility API.
- Monitor global keyboard events if configured (keyboard event kind).

These are powerful system permissions. mimi uses them **only** for event detection and hook execution and does **not** record, transmit, or log the content of UI elements or keystrokes beyond what is needed for its matching rules.

### No Network Access

mimi does **not**:

- Make any outbound network connections.
- Send telemetry, analytics, or crash reports.
- Contact update servers or phone home.

All communication is strictly local — the CLI and daemon talk over a **Unix domain socket** created with owner-only permissions (`0600`) in the system temporary directory.

### IPC

The CLI communicates with the running daemon via a local Unix socket using a JSON-based message protocol. The socket is not exposed over the network. Only the local user can connect to it.

### CGo / Objective-C Bridge

mimi uses CGo to call macOS Objective-C APIs for event observers (workspace, power, audio, USB, network, display, clipboard, AX). The bridge code lives in `internal/bridge/` and is the primary attack surface for memory-safety issues. If you find a vulnerability in this layer (buffer overflow, use-after-free, etc.), please report it.

### Hook Execution

mimi executes shell commands defined in its configuration via `exec.Command` with a configurable timeout. Hook commands run with the same privileges as the daemon (the logged-in user). Care should be taken not to introduce untrusted content into hook commands or environment variables.

### Dependencies

mimi has a small dependency footprint. Go modules are managed via `go.sum` for integrity verification. We recommend reviewing dependency updates carefully, especially those touching CGo or system API bindings.

---

## Scope

The following are **in scope** for security reports:

- Privilege escalation through the accessibility permission.
- IPC socket vulnerabilities (e.g. unauthorized command execution).
- Memory safety issues in the CGo/Objective-C bridge.
- Information disclosure (e.g. logging sensitive UI content or hook output).
- Dependency vulnerabilities with a realistic exploit path.
- Injection attacks via hook commands or event filter rules.

The following are **out of scope**:

- Issues that require the attacker to already have local root access.
- Denial-of-service against the local daemon (the user can simply restart it).
- Vulnerabilities in macOS itself or its notification/accessibility APIs.
- Social engineering attacks.

---

Thank you for helping keep mimi and its users safe.
