# Testing Patterns

## Test File Naming

- Unit tests: `*_test.go` (no build tag required)
- macOS integration tests: `*_integration_test.go` (tagged `//go:build integration`)

## Test Function Naming

```go
func TestService_Method(t *testing.T)
func TestService_Method_EdgeCase(t *testing.T)
```

## Test Types

| Type        | Command                 | Purpose                                                                        |
| ----------- | ----------------------- | ------------------------------------------------------------------------------ |
| Unit        | `just test-unit`        | Business logic, algorithms, config validation with mocks                        |
| Integration | `just test-integration` | Real macOS APIs, file system (tagged `//go:build integration`)                   |

## When to Use Each Type

| Scenario           | Test Type   | Example                            |
| ------------------ | ----------- | ---------------------------------- |
| Business logic     | Unit        | Event kind matching, hook filtering |
| Config validation  | Unit        | TOML parsing, field validation     |
| Platform API calls | Integration | Observer lifecycle, CGO bridge     |
| File operations    | Integration | Config loading, log writing        |

## Test Structure

### Arrange-Act-Assert

```go
func TestHookFilter(t *testing.T) {
  registry := NewRegistry()
  hooks := registry.HooksFor(evt)
  if len(hooks) != 1 {
    t.Fatalf("expected 1 hook, got %d", len(hooks))
  }
}
```

### Table-Driven Tests

```go
func TestValidate(t *testing.T) {
  tests := []struct {
    name    string
    input   string
    wantErr bool
  }{
    {"valid input", "valid", false},
    {"empty input", "", true},
  }

  for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
      err := Validate(tt.input)
      if (err != nil) != tt.wantErr {
        t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
      }
    })
  }
}
```

## Integration Tests

Integration tests that depend on native macOS APIs must use build tags:

```go
//go:build integration

package observe_test

import "testing"

func TestWorkspaceObserver(t *testing.T) {
  // ...
}
```

### Recorded window baseline

`internal/baseline` holds `window_baseline.json`: the frames macOS actually
produces for every `resize_window` preset, anchor and margin state, and the
window `focus_window` picks in each direction. The integration recorder in that
package drives the real actions to produce it.

Two properties matter when changing it:

- **It only ever drives windows it opened itself.** The recorder launches its
  own throwaway TextEdit instance and matches windows by process ID. Any step
  that would otherwise reach a window it did not create skips instead.
- **It skips, never fails, on a machine that cannot run it** — no Accessibility
  permission (CI), a locked screen, or a display other than the one the
  recording was captured on.

The recorder must launch its helper application before the first window
enumeration. `NSWorkspace` refreshes its running-application list from the run
loop, which a test binary never spins, so an application launched after that
list is first read stays invisible to the action layer for the rest of the
process.

Re-record after changing a case, or to capture the baseline on your own display:

```bash
MIMI_RECORD_BASELINE=1 go test -tags=integration -run TestWindowBaseline_ResizeAndFocus ./internal/baseline
```

Unit tests consume the same recording through `baseline.Load`, so a pure
reimplementation of the geometry can be checked against observed behavior rather
than against a reading of the old code.

## Test Commands

- `just test-unit` — Runs unit tests
- `just test-integration` — Runs integration tests (`-tags=integration`)
- `just test-race` — Runs all tests with race detection
