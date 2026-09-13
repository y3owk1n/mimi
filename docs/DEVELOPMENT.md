# Development guide

## Prerequisites

- macOS (required for CGO/Objective-C)
- Go 1.26.4 or later (`go.mod`)
- [just](https://github.com/casey/just) (build system)
- [devbox](https://www.jetify.com/devbox) (optional, provisions Go, just, golangci-lint and clang-format)

```bash
devbox shell
just build
```

---

## Project layout

```
cmd/mimi/              CLI binary
cmd/genman/            Man page generator
internal/
  action/              mimi action dispatch, queries, the Desktop seam
  geometry/            Pure window geometry
  native/              All Obj-C + CGO in the core: AX window wrappers,
                       Mission Control operations, observers, overlays
  ipc/                 Unix socket between CLI and daemon
  observe/             Hook daemon event routing
  events/              Event kinds and bus
  hooks/               Hook registry and executor
  tiling/              Layout program engine
  border/              Window borders
  dropzone/            Drop preview while dragging
  stackbar/            Stack indicator
  config/              TOML config
  daemon/              Daemon lifecycle
  service/             launchd service management
  logging/             Logger and event log
  errors/              Coded errors
  paths/               Path helpers
  permissions/         Accessibility checks
  systray/             Menu bar UI
  baseline/            Recorded window behaviour for tests
```

[ARCHITECTURE.md](ARCHITECTURE.md#package-layout) describes each package.

---

## Build commands

```bash
just build          # build bin/mimi
just test           # unit + integration tiers, one pass
just test-unit      # unit tier only
just test-race      # both tiers under the race detector
just lint           # golangci-lint
just fmt            # format Go + Objective-C
just fmt-check      # check Objective-C formatting
just vet            # go vet, untagged and -tags=integration
just bundle         # build build/Mimi.app
just genman         # generate man pages
```

---

## Adding a CLI command

1. Create `cmd/mimi/cmd/<name>.go` with a constructor that returns a Cobra command using `RunE`.
2. Register it with `AddCommand` in its parent: `newRootCmd` in `root.go` for a top-level command, or `newActionCmd` in `action.go` for an action.
3. Put business logic in `internal/`.
4. Document it in `docs/CLI.md`.

---

## Native code

Objective-C lives in:

- `internal/native/` for window and space actions, hook daemon observers, and the border, drop zone and stack bar overlays
- `internal/systray/` for the menu bar UI
- `internal/permissions/` for Accessibility permission prompts

Format with `just fmt`. See [OBJECTIVE_C.md](go/OBJECTIVE_C.md).

---

## Testing

```bash
just test-unit          # unit tier, no Accessibility grant needed
just test-integration   # every test with -tags=integration
```

The integration tier lives in files named `*_integration_test.go` behind `//go:build integration`. Today it covers the recorded window baseline in `internal/baseline` and the real `launchctl` in `internal/service`. A test that drives the desktop skips, rather than fails, on a machine that cannot run it, such as one without an Accessibility grant. Do not add a `//go:build !integration` constraint, because `just test` runs only the tagged pass and would never run that file. See [TESTING_PATTERNS.md](testing/TESTING_PATTERNS.md).
