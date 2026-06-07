# Mimi Coding Standards

This document defines the coding standards and conventions for the mimi project. Following these standards ensures the codebase appears written by a single developer and maintains consistency across all files.

---

## Table of Contents

- [Quick Reference](#quick-reference)
- [General Standards](#general-standards)
- [Logging Standards](#logging-standards)
- [Error Handling](#error-handling)
- [Documentation Standards](#documentation-standards)
- [Git Commit Standards](#git-commit-standards)
- [Pre-commit Checklist](#pre-commit-checklist)
- [References](#references)

---

## Quick Reference

- [Go CONVENTIONS.md](./go/CONVENTIONS.md) — Go code style, imports, naming, error handling
- [Go OBJECTIVE_C.md](./go/OBJECTIVE_C.md) — .h/.m files, naming, memory management
- [TESTING_PATTERNS.md](./testing/TESTING_PATTERNS.md) — Test file naming, unit vs integration, table-driven tests

---

## General Standards

### File Formatting

All files must follow these basic formatting rules (enforced by `.editorconfig`):

- **Character encoding**: UTF-8
- **Line endings**: LF (Unix-style)
- **Indentation**: Tabs (width 4 spaces when displayed)
- **Trailing whitespace**: None
- **Final newline**: Required

### File Organization

```
mimi/
├── cmd/
│   └── mimi/           # Application entry points
├── internal/
│   ├── config/         # Configuration management
│   ├── daemon/         # Daemon lifecycle
│   ├── errors/         # Structured error types
│   ├── events/         # Event types + pub-sub bus
│   ├── hooks/          # Hook registry + executor
│   ├── logging/        # Structured logging
│   ├── native/         # Objective-C + CGO bridge
│   ├── observe/        # Go-side event routing
│   ├── action/         # CLI action dispatch
│   ├── window/         # AX window wrappers
│   ├── space/          # Mission Control operations
│   └── permissions/    # Accessibility permission checks
├── configs/            # Embedded default config
├── docs/               # Documentation
└── nix/                # Nix packaging
```

### Naming Conventions

- **Directories**: lowercase, underscore-separated
- **Files**: lowercase, underscore-separated
- **Test files**: `*_test.go`, `*_integration_test.go`

---

## Logging Standards

### Logger

Mimi uses `*zap.SugaredLogger` from `go.uber.org/zap`. Constructors that accept a logger should tolerate `nil` by falling back to `zap.NewNop()`.

### Log Levels

- `debug`: High-volume diagnostic info — event routing, window polling cycles, AX observer installs
- `info`: Daemon lifecycle — startup, shutdown, config load, mode activation
- `warn`: Actionable degradation — missing accessibility permission, config reload failure
- `error`: Failed operations — include `zap.Error(err)` and relevant context

### Fields

Prefer structured fields over interpolated messages:

```go
logger.Warnw("config reload failed", "err", err)
logger.Infow("event", "kind", evt.Kind, "app", evt.AppName)
```

Do not log sensitive or unbounded payloads — log counts, lengths, IDs, booleans, and durations instead.

---

## Error Handling

Use the `derrors` package for structured errors:

```go
import derrors "github.com/y3owk1n/mimi/internal/errors"

// Create new error
return derrors.New(derrors.CodeInternal, "something went wrong")

// Wrap existing error
return derrors.Wrapf(err, derrors.CodeConfigIOFailed, "reading config")
```

Available error codes: `CodeAccessibilityDenied`, `CodeAccessibilityFailed`, `CodeInvalidConfig`, `CodeInvalidInput`, `CodeActionFailed`, `CodeContextCanceled`, `CodeTimeout`, `CodeInternal`, `CodeLoggingFailed`, `CodeConfigIOFailed`, `CodeSerializationFailed`, `CodeBridgeFailed`, `CodeNotSupported`.

---

## Documentation Standards

### Code Comments

**Do comment:**
- Complex algorithms or logic
- Non-obvious performance optimizations
- Workarounds for bugs or limitations
- Public APIs and exported symbols

**Don't comment:**
- Obvious code
- Redundant information already in the code
- Outdated information (update or remove)

### Package Documentation

Every package should have a `doc.go` file with package-level documentation.

---

## Git Commit Standards

### Format

```
<type>: <subject>

<body>

<footer>
```

### Types

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `test`: Adding or updating tests
- `chore`: Build process, dependencies, etc.

---

## Pre-commit Checklist

- [ ] Code formatted (`just fmt`)
- [ ] Linters pass (`just lint`)
- [ ] Tests pass (`just test`)
- [ ] Build succeeds (`just build`)
- [ ] Documentation updated if needed
- [ ] Commit message follows standards

---

## References

- [Effective Go](https://golang.org/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- [Apple Coding Guidelines for Cocoa](https://developer.apple.com/library/archive/documentation/Cocoa/Conceptual/CodingGuidelines/CodingGuidelines.html)
