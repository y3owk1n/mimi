# Development Guide

Contributing to mimi: build instructions, architecture overview, and development workflow.

---

## Table of Contents

- [Quick Start](#quick-start)
- [Development Setup](#development-setup)
- [Building](#building)
- [Testing](#testing)
- [Linting](#linting)
- [Code Standards](#code-standards)
- [Architecture Overview](#architecture-overview)
- [Contributing](#contributing)

---

## Quick Start

```bash
# 1. Clone and setup
git clone https://github.com/y3owk1n/mimi.git
cd mimi

# 2. Install dependencies
brew install just golangci-lint

# 3. Build and run
just build
./bin/mimi start

# 4. Test it works
mimi status
```

---

## Development Setup

### Prerequisites

- **Go 1.26+** — [Install Go](https://golang.org/dl/)
- **Xcode Command Line Tools** — `xcode-select --install`
- **Just** — Command runner ([Install](https://github.com/casey/just))

    ```bash
    brew install just
    ```

- **golangci-lint** — Linter ([Install](https://golangci-lint.run/usage/install/))

    ```bash
    brew install golangci-lint
    ```

### Development Environment

This project includes a `devbox.json` for [Devbox](https://www.jetify.com/devbox):

```bash
# Enter the dev environment
devbox shell

# Or use direnv for automatic activation
brew install direnv
# Add to ~/.zshrc: eval "$(direnv hook zsh)"
```

Devbox provides Go 1.26, gopls, golangci-lint, just, and clang-tools.

### Verify Setup

```bash
go version      # Should be 1.26+
just --version
golangci-lint --version
```

---

## Building

Mimi uses [Just](https://github.com/casey/just) as the build system.

### Common Build Commands

| Command        | Description                          |
| -------------- | ------------------------------------ |
| `just build`   | Development build to `bin/mimi`      |
| `just release` | Optimised build (stripped, trimpath) |
| `just bundle`  | Build `build/Mimi.app` bundle        |
| `just clean`   | Remove `bin/` and `build/`           |

### Manual Build

```bash
go build -o bin/mimi ./cmd/mimi
```

### Build with Version Info

```bash
VERSION=$(git describe --tags --always --dirty)
go build \
  -ldflags="-s -w -X github.com/y3owk1n/mimi/cmd/mimi/cmd.Version=$VERSION" \
  -o bin/mimi ./cmd/mimi
```

### CGO Requirement

Mimi requires `CGO_ENABLED=1` (default on macOS) for the Objective-C bridge. The build system handles this automatically:

```
{{ if os() == "windows" { "CGO_ENABLED=0" } else { "CGO_ENABLED=1" } }} go build ...
```

---

## Testing

Mimi has a small test suite with unit and integration tests.

### Test Commands

| Command                 | Description                                 |
| ----------------------- | ------------------------------------------- |
| `just test`             | Run all tests (unit + integration)          |
| `just test-unit`        | Run unit tests                              |
| `just test-integration` | Run integration tests (`-tags=integration`) |
| `just test-race`        | Run all tests with race detection           |

### Integration Tests

Integration tests require a running macOS session and are tagged with `//go:build integration`:

```bash
just test-integration
```

---

## Linting

Mimi uses `golangci-lint` with a comprehensive linter configuration.

### Lint Commands

| Command     | Description       |
| ----------- | ----------------- |
| `just fmt`  | Format Go + Obj-C |
| `just lint` | Run all linters   |
| `just vet`  | Run `go vet`      |

### Auto-Fix

```bash
golangci-lint run --fix
```

---

## Code Standards

- **Go**: Follow standard Go idioms and `golangci-lint` rules
- **Objective-C**: Format with `clang-format` (included in `just fmt`)
- **Imports**: Group standard library, third-party, and local imports with blank lines
- **Errors**: Use structured errors from `internal/errors/` with error codes
- **Logging**: Use `*zap.SugaredLogger` with structured fields

### Package Layout

```
cmd/mimi/                   # Entry point
internal/
├── events/                 # Event types + pub-sub bus
├── config/                 # TOML config loading + validation
├── hooks/                  # Hook registry + executor
├── observers/              # Go-side observer orchestration
│   └── cgo_bridge/         # CGo + Objective-C native layer
├── daemon/                 # Daemon lifecycle
├── permissions/            # Accessibility permission checks
├── logging/                # Structured logging
└── errors/                 # Error types
configs/                    # Embedded default config
```

---

## Architecture Overview

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full architectural reference: event flow, component diagrams, CGO bridge structure, and daemon lifecycle.

---

## Contributing

### Development Workflow

1. Fork and clone the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Make changes following the code standards
4. Test thoroughly: `just test && just lint && just build`
5. Commit with a descriptive message
6. Push and open a PR

### Pre-commit Checklist

- [ ] Code formatted (`just fmt`)
- [ ] Linters pass (`just lint`)
- [ ] Tests pass (`just test`)
- [ ] Build succeeds (`just build`)
- [ ] Documentation updated if needed

### Adding New Events

1. Add a new `EventKind` constant in `internal/events/types.go`
2. Register it in the kind-to-int map in `internal/observers/cgo_bridge/bridge.go`
3. Implement the Objective-C observer in `internal/observers/cgo_bridge/system_events.m`
4. Start/stop it in `InitCocoaApp` / `Start` / respective observer functions
5. Add the hook key to `internal/hooks/registry.go` (`HooksFor` matching)
6. Add the TOML key to `configs/embed.go` default config
7. Document it in [CONFIGURATION.md](CONFIGURATION.md)

### Adding New CLI Commands

1. Create command file in `cmd/mimi/cmd/`
2. Register in `cmd/mimi/cmd/root.go` (add to root command)
3. Document in [CLI.md](CLI.md)
