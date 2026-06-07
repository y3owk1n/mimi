# Development Guide

## Prerequisites

- macOS (required for CGO/Objective-C)
- Go 1.26+
- [just](https://github.com/casey/just) (build system)
- [devbox](https://www.jetify.com/devbox) (optional, recommended)

```bash
devbox shell
just build
```

---

## Project Layout

```
cmd/mimi/              CLI binary
internal/
  action/              mimi action dispatch
  window/              AX window wrappers
  space/               Mission Control operations
  native/              All Obj-C + CGO (actions and observers)
  observe/             Hook daemon event routing
  hooks/               Hook registry and executor
  config/              TOML config
  daemon/              Daemon lifecycle
  permissions/         Accessibility checks
  systray/             Menu bar UI
```

---

## Build Commands

```bash
just build          # build bin/mimi
just test           # run all tests
just test-unit      # unit tests only
just lint           # golangci-lint
just fmt            # format Go + Objective-C
just bundle         # build Mimi.app
just genman         # generate man pages
```

---

## Adding a CLI Command

1. Create `cmd/mimi/cmd/<name>.go` with a Cobra command using `RunE`
2. Register in `root.go` `init()`
3. Put business logic in `internal/`
4. Document in `docs/CLI.md`

---

## Native Code

Objective-C lives in:

- `internal/native/` — window/space actions and hook daemon observers
- `internal/systray/` — menu bar UI
- `internal/permissions/` — accessibility permission prompts

Format with `just fmt`. See [OBJECTIVE_C.md](go/OBJECTIVE_C.md).

---

## Testing

```bash
just test-unit      # pure Go tests (no macOS APIs)
```

Integration tests for native APIs are not yet implemented. When adding them, use `//go:build integration` and name files `*_integration_test.go`.
