# Testing patterns

## Test file naming

- Unit tests: `*_test.go` (no build tag)
- macOS integration tests: `*_integration_test.go` (tagged `//go:build integration`)

## Test function naming

```go
func TestService_Method(t *testing.T)
func TestService_Method_EdgeCase(t *testing.T)
```

## Test types

| Type        | Command                 | Purpose                                                                  |
| ----------- | ----------------------- | ------------------------------------------------------------------------ |
| Unit        | `just test-unit`        | Business logic, algorithms, config loading and validation                |
| Integration | `just test-integration` | Real macOS APIs and system tools (tagged `//go:build integration`)       |

A build tag adds files to a build and never narrows the build to them. The
`integration` build therefore already contains the unit tier, and
`just test-integration` runs the integration tier on top of the unit tests, not
the tagged tests alone.

## When to use each type

| Scenario           | Test type   | Example                                    |
| ------------------ | ----------- | ------------------------------------------ |
| Business logic     | Unit        | Event kind matching, hook filtering        |
| Config validation  | Unit        | TOML parsing, field validation             |
| File operations    | Unit        | Config loading from `t.TempDir()`          |
| Platform API calls | Integration | Window enumeration, focus, resize          |
| System tools       | Integration | `launchctl` through the service launcher   |

## Test structure

### Arrange-act-assert

```go
func TestHookFilter(t *testing.T) {
	registry := NewRegistry()
	hooks := registry.HooksFor(evt.Kind)
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}
}
```

### Table-driven tests

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

## Integration tests

Tests that depend on native macOS APIs or system tools must carry the build tag:

```go
//go:build integration

package baseline_test

import "testing"

func TestEnumeration_SeesAnApplicationLaunchedAfterTheFirstEnumeration(t *testing.T) {
	// ...
}
```

### Recorded window baseline

`internal/baseline` holds `window_baseline.json`, which records the frames
macOS produces for every `resize_window` preset, anchor, and margin state, and
the window `focus_window` picks in each direction. The integration recorder in
that package drives the real actions to produce it.

Two properties matter when changing it:

- The recorder drives only windows it opened itself. It launches its own
  throwaway TextEdit instance and matches windows by process ID. A step that
  would reach a window it did not create skips instead.
- It skips, and never fails, on a machine that cannot run it. That covers a
  missing Accessibility permission (CI), a locked screen, and a display other
  than the one the recording was captured on.

Window enumeration derives its applications from the window list, so the
recorder can launch its helper at any point.
`TestEnumeration_SeesAnApplicationLaunchedAfterTheFirstEnumeration` covers this.

Re-record after changing a case, or to capture the baseline on your own display:

```bash
MIMI_RECORD_BASELINE=1 go test -tags=integration -run TestWindowBaseline_ResizeAndFocus ./internal/baseline
```

Unit tests read the same recording through `baseline.Load`. A pure
reimplementation of the geometry is then checked against observed behavior, not
against a reading of the old code.

## Test commands

- `just test`: runs every test once, as a single tagged pass that covers both
  tiers
- `just test-unit`: runs the unit tier alone, with no Accessibility grant needed
- `just test-integration`: runs every test with the integration tier enabled
  (`-tags=integration`)
- `just test-race`: runs every test once under the race detector
- `just test-race-unit`: runs the unit tier alone under the race detector
- `just test-all`: runs `just test` then `just test-race` (what CI runs)

`just vet` vets both builds, untagged and `-tags=integration`, so a mistake in a
`*_integration_test.go` file is caught even on a machine that cannot run the
tier.
