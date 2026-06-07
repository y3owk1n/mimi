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

## Test Commands

- `just test-unit` — Runs unit tests
- `just test-integration` — Runs integration tests (`-tags=integration`)
- `just test-race` — Runs all tests with race detection
