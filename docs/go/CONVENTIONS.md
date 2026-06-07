# Go Conventions

## Package Organization

### Package Names

- Use short, lowercase, single-word names when possible
- Avoid underscores, hyphens, or mixed caps

```go
package config
package events
package hooks
```

### Package Documentation

Every package should have a `doc.go` file with package-level documentation:

```go
// Package config provides TOML configuration loading and validation for mimi.
package config
```

## File Structure

1. Package declaration
2. Imports (organized by `goimports`)
3. Constants
4. Type definitions
5. Constructor functions
6. Methods (grouped by receiver type)
7. Helper functions

## Imports

Organized by `goimports` into three groups:

1. Standard library
2. External packages
3. Internal packages

Use blank lines between groups:

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
- Receiver names: consistent single-letter (e.g., `o` for `WorkspaceObserver`, `w` for `Watcher`)

## Function Parameters

- `context.Context` first parameter when needed for cancellable operations
- Required parameters before optional

```go
func (ex *Executor) Run(ctx context.Context, sub <-chan events.Event)
```

## Return Values

- Return errors as the last value
- Use named return values sparingly

```go
func (l *Loader) Load(path string) (*Config, error) {
  cfg, err := l.parse(path)
  if err != nil {
    return nil, err
  }
  return cfg, nil
}
```

## Error Handling

Use the `derrors` package for structured errors:

```go
import derrors "github.com/y3owk1n/mimi/internal/errors"

// Create new error
return derrors.New(derrors.CodeInvalidConfig, "config validation failed")

// Wrap existing error
return derrors.Wrapf(err, derrors.CodeConfigIOFailed, "reading config")
```

## Context

- Accept `context.Context` as first parameter for cancellable operations
- Don't store context in structs

```go
func (w *Watcher) Run(ctx context.Context) error {
  select {
  case <-ctx.Done():
    return nil
  case ev := <-w.fileWatcher.Events:
    // ...
  }
}
```

## Concurrency

### Mutex Usage

- Use `sync.RWMutex` for read-heavy workloads
- Use `sync.Mutex` for write-heavy or simple cases
- Always defer unlock immediately after lock

```go
func (s *Service) Get(id string) (*Item, error) {
  s.mu.RLock()
  defer s.mu.RUnlock()
  return s.cache[id], nil
}
```

### Goroutines

- Use a semaphore pattern (`chan struct{}`) to limit concurrent goroutines
- Always provide a mechanism for graceful shutdown via context cancellation

## Comments

- Comment public APIs and exported symbols
- Use complete sentences with proper punctuation
- Explain _why_ for non-obvious code, not _what_

```go
// Pre-allocate slice capacity to avoid reallocations during env var building.
// Typical event has 9 base vars plus Extra entries.
vars := make([]string, 0, baseEnvVarCount+len(evt.Extra))
```

## Performance

### Pre-allocation

```go
vars := make([]string, 0, expectedCount)
envMap := make(map[string]string, len(env))
```

### String Building

```go
var b strings.Builder
b.WriteString("mimi_")
b.WriteString(key)
b.WriteString("=")
b.WriteString(value)
return b.String()
```

## macOS-Specific Conventions

mimi is macOS-only, so no cross-platform build tags or platform factories are needed. CGo code lives in `internal/native/`.

## See Also

- [TESTING_PATTERNS.md](../testing/TESTING_PATTERNS.md)
- [OBJECTIVE_C.md](./OBJECTIVE_C.md)
