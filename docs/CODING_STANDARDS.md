# mimi coding standards

This document defines the coding standards and conventions for the mimi project.

---

## Table of contents

- [Quick reference](#quick-reference)
- [General standards](#general-standards)
- [Logging standards](#logging-standards)
- [Error handling](#error-handling)
- [Documentation standards](#documentation-standards)
- [Git commit standards](#git-commit-standards)
- [Pre-commit checklist](#pre-commit-checklist)
- [References](#references)

---

## Quick reference

- [Go CONVENTIONS.md](./go/CONVENTIONS.md): Go code style, imports, naming, error handling
- [Go OBJECTIVE_C.md](./go/OBJECTIVE_C.md): .h/.m files, naming, memory management
- [TESTING_PATTERNS.md](./testing/TESTING_PATTERNS.md): test file naming, unit and integration tiers, table-driven tests

---

## General standards

### File formatting

`.editorconfig` sets these rules for all files:

- Character encoding: UTF-8
- Line endings: LF
- Indentation: tabs, displayed 4 wide
- Trailing whitespace: trimmed
- Final newline: required

### File organization

```
mimi/
├── cmd/
│   ├── mimi/           # CLI entry point and cobra commands
│   └── genman/         # Man page generator
├── internal/
│   ├── action/         # CLI action dispatch
│   ├── baseline/       # Recorded macOS window behavior (test oracle)
│   ├── border/         # Window borders
│   ├── config/         # Configuration management
│   ├── daemon/         # Daemon lifecycle
│   ├── dropzone/       # Drop zone preview while dragging a tiled window
│   ├── errors/         # Structured error types
│   ├── events/         # Event types + pub-sub bus
│   ├── geometry/       # Pure window geometry (no dependencies)
│   ├── hooks/          # Hook registry + executor
│   ├── ipc/            # Unix socket between the CLI and the daemon
│   ├── logging/        # Structured logging
│   ├── native/         # Objective-C + CGO bridge: AX window wrappers,
│   │                   # Mission Control operations, observers
│   ├── observe/        # Go-side event routing
│   ├── paths/          # Path helpers
│   ├── permissions/    # Accessibility permission checks (CGO)
│   ├── service/        # launchd service management
│   ├── stackbar/       # Stacked-window cards
│   ├── systray/        # Menu bar UI (CGO)
│   └── tiling/         # Tiling engine and layout programs
├── configs/            # Embedded default config
├── docs/               # Documentation
├── examples/           # Example tiling layouts
├── nix/                # Nix packaging
└── resources/          # App bundle Info.plist and entitlements
```

### Naming conventions

- Directories: lowercase, underscore-separated
- Files: lowercase, underscore-separated
- Test files: `*_test.go`, `*_integration_test.go`

---

## Logging standards

### Logger

mimi uses `*zap.SugaredLogger` from `go.uber.org/zap`. A constructor that accepts a logger must tolerate `nil` by falling back to `zap.NewNop()`.

### Log levels

- `debug`: high-volume diagnostics, such as event routing and AX observer installs
- `info`: daemon lifecycle, such as startup, shutdown, and config reload
- `warn`: degradation the user can act on, such as a missing Accessibility permission or a failed config reload
- `error`: failed operations, with the error passed as a field (`"err", err`) and the IDs needed to find the failure

### Fields

Use structured fields instead of interpolated messages:

```go
logger.Warnw("config reload failed", "trigger", trigger, "err", err)
logger.Errorw("hook failed", "kind", evt.Kind, "index", hookIndex, "exit", err)
```

Never log window titles, hook command contents, or other user payloads. Log counts, lengths, IDs, booleans, and durations instead.

---

## Error handling

Use the `derrors` package for structured errors. Never return a bare `errors.New` across a package boundary.

```go
import derrors "github.com/y3owk1n/mimi/internal/errors"

// Create new error
return derrors.New(derrors.CodeInternal, "something went wrong")

// Wrap existing error
return derrors.Wrapf(err, derrors.CodeConfigIOFailed, "reading config")
```

Available error codes: `CodeAccessibilityDenied`, `CodeAccessibilityFailed`, `CodeInvalidConfig`, `CodeInvalidInput`, `CodeActionFailed`, `CodeContextCanceled`, `CodeTimeout`, `CodeInternal`, `CodeLoggingFailed`, `CodeConfigIOFailed`, `CodeSerializationFailed`, `CodeBridgeFailed`, `CodeDaemonUnavailable`, `CodeIPCFailed`, `CodeProtocolMismatch`, `CodeServiceFailed`, `CodeNotSupported`.

`internal/errors/coding_standards_test.go` checks this list against the constants in `errors.go`, so update both together.

---

## Documentation standards

### Code comments

Comment:

- Complex algorithms or logic
- Non-obvious performance optimizations
- Workarounds for bugs or limitations
- Public APIs and exported symbols

Do not comment:

- Obvious code
- What the code already states
- Outdated information (update or remove it)

### Package documentation

Every package should have a `doc.go` file with package-level documentation.

---

## Git commit standards

### Format

```
<type>(<optional scope>): <subject>

<body>

<footer>
```

The repo squash-merges with the PR title as the commit subject, so the PR title follows this format too.

### Types

Shown in the changelog:

- `feat`: New feature
- `fix`: Bug fix
- `perf`: Performance improvements
- `improve`: Improvement to existing behavior
- `experiment`: Experimental feature
- `revert`: Revert of an earlier commit
- `docs`: Documentation changes

Hidden from the changelog:

- `refactor`: Code refactoring
- `style`: Code style changes (formatting, etc.)
- `test`: Adding or updating tests
- `ci`: CI workflows
- `build`: Build system
- `chore`: Dependencies, tooling, other upkeep

`release-please-config.json` is the source for this split.

---

## Pre-commit checklist

- [ ] Code formatted (`just fmt`)
- [ ] Linters pass (`just lint`)
- [ ] Tests pass (`just test`)
- [ ] Build succeeds (`just build`)
- [ ] Documentation updated if needed
- [ ] Commit message follows standards

CI also runs `just fmt-check`, `just vet`, and `just test-all`.

---

## References

- [Effective Go](https://golang.org/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- [Apple Coding Guidelines for Cocoa](https://developer.apple.com/library/archive/documentation/Cocoa/Conceptual/CodingGuidelines/CodingGuidelines.html)
