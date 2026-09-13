# Go conventions

## Package organization

### Package names

- Use short, lowercase, single-word names when possible
- Avoid underscores, hyphens, or mixed caps

```go
package config
package events
package hooks
```

### Package documentation

Every package should have a `doc.go` file with package-level documentation:

```go
// Package config provides TOML configuration loading and validation for mimi.
package config
```

## File structure

1. Package declaration
2. Imports (grouped by `gci`)
3. Constants
4. Type definitions
5. Constructor functions
6. Methods (grouped by receiver type)
7. Helper functions

## Imports

`golangci-lint fmt` runs `gci`, which sorts imports into three groups separated by blank lines:

1. Standard library
2. External packages
3. Internal packages (`github.com/y3owk1n/mimi/...`)

```go
import (
	"context"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/events"
)
```

## Naming

- Packages: lowercase, short, descriptive
- Variables: camelCase local, PascalCase exported
- Constants: PascalCase exported, camelCase unexported
- Receiver names: short and consistent across a type's methods (e.g., `w` for `config.Watcher`, `r` for `hooks.Registry`, `ex` for `hooks.Executor`)

## Function parameters

- `context.Context` is the first parameter when the operation can be canceled
- Required parameters come before optional ones

```go
func (ex *Executor) Run(ctx context.Context, sub events.Subscriber)
```

## Return values

- Return errors as the last value
- Use named return values sparingly

```go
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, derrors.Wrapf(err, derrors.CodeConfigIOFailed, "reading config")
	}
	// ...
}
```

## Error handling

Use the `derrors` package for structured errors:

```go
import derrors "github.com/y3owk1n/mimi/internal/errors"

// Create new error
return derrors.New(derrors.CodeInvalidConfig, "config validation failed")

// Wrap existing error
return derrors.Wrapf(err, derrors.CodeConfigIOFailed, "reading config")
```

## Context

- Accept `context.Context` as the first parameter for cancelable operations
- Don't store a context in a struct

```go
func (w *Watcher) Run(ctx context.Context) error {
	// ...
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-fileWatcher.Events:
			// ...
		}
	}
}
```

## Concurrency

### Mutex usage

- Use `sync.RWMutex` for read-heavy workloads
- Use `sync.Mutex` for write-heavy or simple cases
- Defer the unlock on the line after the lock

```go
func (s *Service) Get(id string) (*Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[id], nil
}
```

### Goroutines

- Limit concurrent goroutines with a semaphore channel (`chan struct{}`), as `hooks.Executor` does for hook workers
- Stop every goroutine when its context is canceled

## Comments

- Comment public APIs and exported symbols
- Write complete sentences with punctuation
- Explain why for non-obvious code, not what

```go
// Pre-allocate slice capacity to avoid reallocations during env var building.
// Every event has 7 base vars plus its Extra entries.
vars := make([]string, 0, baseEnvVarCount+len(evt.Extra))
```

## Performance

### Pre-allocation

```go
vars := make([]string, 0, expectedCount)
envMap := make(map[string]string, len(env))
```

### String building

```go
var b strings.Builder
b.WriteString("mimi_")
b.WriteString(key)
b.WriteString("=")
b.WriteString(value)
return b.String()
```

## macOS-specific conventions

mimi is macOS-only, so it needs no cross-platform build tags or platform factories. CGO code lives only in `internal/native/`, `internal/systray/`, and `internal/permissions/`. No other package imports `"C"`.

## See also

- [TESTING_PATTERNS.md](../testing/TESTING_PATTERNS.md)
- [OBJECTIVE_C.md](./OBJECTIVE_C.md)
