# Contributing to mimi

mimi is a macOS window and space utility. Code, docs, bug reports, config examples, and ideas are all welcome.

---

## Table of contents

- [Code of conduct](#code-of-conduct)
- [Getting started](#getting-started)
- [Development setup](#development-setup)
- [Making changes](#making-changes)
- [Commit messages](#commit-messages)
- [Pull requests](#pull-requests)
- [Testing](#testing)
- [Code style](#code-style)
- [Good first contributions](#good-first-contributions)
- [Reporting bugs](#reporting-bugs)
- [Feature requests](#feature-requests)

---

## Code of conduct

This project follows the [Code of Conduct](CODE_OF_CONDUCT.md). By participating you agree to uphold it. Report unacceptable behavior through [GitHub Issues](https://github.com/y3owk1n/mimi/issues) or by contacting [@y3owk1n](https://github.com/y3owk1n) directly.

---

## Getting started

1. Search existing issues to see whether someone is already working on the same thing.
2. Open an issue before starting a non-trivial change, so the approach is agreed before you write code.
3. Keep PRs small and focused.

---

## Development setup

### Prerequisites

- Go 1.26+ ([install Go](https://golang.org/dl/))
- Xcode Command Line Tools: `xcode-select --install`
- [just](https://github.com/casey/just), the command runner: `brew install just`
- golangci-lint: `brew install golangci-lint`
- clang-format, which `just fmt` and `just fmt-check` run on the Objective-C files: `brew install clang-format`

`oku sync && oku allow` installs pinned versions of all of these except the Xcode tools, from `oku.toml`, with [oku](https://github.com/y3owk1n/oku). CI installs the same versions with the setup actions.

### Clone and verify

```bash
git clone https://github.com/y3owk1n/mimi.git
cd mimi
go version          # Should be 1.26+
just --version
golangci-lint --version
just --list         # See all available commands
```

For full details see:

- [Development Guide](docs/DEVELOPMENT.md)
- [System Architecture](docs/ARCHITECTURE.md)

---

## Making changes

1. Fork the repository and clone your fork.
2. Create a branch from `main`, named `<type>/<short-summary>`:

    ```bash
    git checkout -b feat/my-feature
    ```

3. Make your changes following the [Coding Standards](docs/CODING_STANDARDS.md).
4. Add or update tests for new or changed behavior.
5. Run the pre-commit checklist:

    ```bash
    just fmt            # Format Go and Objective-C
    just lint           # Run golangci-lint
    just test           # Run unit and integration tests, each once
    just build          # Verify build
    ```

6. Commit using [conventional commits](#commit-messages).
7. Push and open a pull request.

---

## Commit messages

mimi uses [Conventional Commits](https://www.conventionalcommits.org/).

Format:

```
<type>(<optional scope>): <subject>
<optional body>
<optional footer>
```

Types:

| Type         | When to use                            | In changelog |
| ------------ | -------------------------------------- | ------------ |
| `feat`       | New feature                            | Yes          |
| `fix`        | Bug fix                                | Yes          |
| `perf`       | Performance improvement                | Yes          |
| `improve`    | Improvement to existing behavior       | Yes          |
| `experiment` | Experimental feature                   | Yes          |
| `revert`     | Revert of an earlier commit            | Yes          |
| `docs`       | Documentation only                     | Yes          |
| `refactor`   | Code restructuring, no behavior change | No           |
| `style`      | Formatting, no logic change            | No           |
| `test`       | Adding or updating tests               | No           |
| `ci`         | CI workflows                           | No           |
| `build`      | Build system                           | No           |
| `chore`      | Dependencies, tooling, other upkeep    | No           |

`release-please-config.json` decides which types reach the changelog.

Examples:

```
feat(action): focus a window by its number
fix(border): follow a dragged window at every step
docs: update configuration reference for workspace hooks
```

---

## Pull requests

- The repo squash-merges with the PR title as the commit subject, so the title is the line that reaches the changelog. Write it in the same conventional commit format (e.g. `feat(action): add space count command`).
- The description explains what changed and why.
- Keep each PR to one logical change.
- Link related issues (e.g. `Closes #123`).
- CI runs `just lint`, `just fmt-check`, `just vet`, `just build`, and `just test-all`. All of them must pass before merge.
- A maintainer reviews every PR.

---

## Testing

mimi splits tests into two tiers:

| Tier        | File pattern            | Command                 | Build tag     |
| ----------- | ----------------------- | ----------------------- | ------------- |
| Unit        | `*_test.go`             | `just test-unit`        | none          |
| Integration | `*_integration_test.go` | `just test-integration` | `integration` |

Guidelines:

- New code needs tests.
- Use table-driven tests where possible.
- Unit tests stay fast and need no Accessibility grant.
- Integration tests drive real macOS APIs and start with `//go:build integration`.

For detailed patterns see [Testing Patterns](docs/testing/TESTING_PATTERNS.md).

---

## Code style

All code follows the [Coding Standards](docs/CODING_STANDARDS.md):

- Go: [Go Conventions](docs/go/CONVENTIONS.md) covers imports, naming, error handling, and receivers.
- Objective-C: [Objective-C Guidelines](docs/go/OBJECTIVE_C.md) covers `.h`/`.m` files, memory management, and naming.
- Format with `just fmt` (runs `golangci-lint fmt` and `clang-format`).
- Lint with `just lint` (runs `golangci-lint`).
- Write godoc comments for all exported symbols.

---

## Good first contributions

- Bug fixes from the [open issues](https://github.com/y3owk1n/mimi/issues)
- Documentation fixes
- Config examples for common setups
- Performance improvements
- Test coverage
- New hook events

---

## Reporting bugs

Open a [GitHub Issue](https://github.com/y3owk1n/mimi/issues/new) with:

1. The macOS version and the mimi version (`mimi --version`).
2. Minimal steps to reproduce.
3. Expected and actual behavior.
4. Logs. Set `log_level = "debug"` under `[settings]` and attach the relevant lines. Logs go to stdout, and also to the `log_file` path when one is set.
5. Your config file, with anything private removed.

See also the [Troubleshooting Guide](docs/TROUBLESHOOTING.md), and the [Security Policy](SECURITY.md) for vulnerability reports.

---

## Feature requests

Open a [GitHub Issue](https://github.com/y3owk1n/mimi/issues/new) or start a [Discussion](https://github.com/y3owk1n/mimi/discussions) that describes:

- What you want.
- Your use case.
- How you picture it working (optional).
