# AGENTS.md — mimi Agent Guide

This file is the shared source of truth for any AI agent working on this repo (Claude Code, Codex, Copilot, Cursor, etc.). `CLAUDE.md` is a symlink to this file — edit here, never the symlink. Personal overrides go in gitignored `AGENTS.local.md` / `CLAUDE.local.md`.

mimi is a macOS window and space utility written in Go, with native bridges into Accessibility, `CGEvent`, and private SkyLight APIs via CGO/Objective-C. It ships a CLI (`mimi action …`) and a hook daemon that fires shell hooks on app, window, and space events. macOS only — there is no cross-platform story.

## Shape

Three execution paths: a direct CLI action, the same action routed over the daemon's Unix socket when it is running, and the hook daemon itself. The CLI silently falls back to direct execution when the socket at `settings.socket_file` is unavailable — a behaviour difference worth remembering when a bug reproduces one way and not the other. `docs/ARCHITECTURE.md` has the full picture.

Space switching and window-to-space moves ride undocumented private APIs and synthetic dock swipes. They are timing-sensitive and may break on macOS updates; treat changes there as user-visible even when the Go surface is unchanged.

## Commands

`just` is the build system; `devbox shell` provisions the toolchain.

```bash
just build          # build bin/mimi
just fmt            # format Go (golangci-lint) + Objective-C (clang-format)
just lint           # golangci-lint run
just vet            # go vet ./...
just test           # unit + integration
just test-unit      # unit only
just bundle         # build build/Mimi.app
just genman         # generate man pages
```

Pre-commit gate: `just fmt && just lint && just test && just build`. CI runs `just lint`, `just fmt-check`, `just vet`, `just build`, and `just test-all` on macOS.

## Conventions

- All Objective-C and CGO lives in `internal/native/`, `internal/systray/`, and `internal/permissions/`. Go packages elsewhere must not open CGO of their own.
- Errors go through `derrors` (`internal/errors`) with a code — `derrors.New(derrors.CodeInvalidInput, …)` / `derrors.Wrapf(err, …)`. Never return a bare `errors.New` across a package boundary.
- Logging is `*zap.SugaredLogger`; constructors tolerate a `nil` logger by falling back to `zap.NewNop()`. Never log window titles, hook command contents, or other user payloads — log counts, IDs, durations, booleans.
- `just fmt-check` gates Objective-C formatting separately from Go. Both must pass.
- Tabs for indentation, LF endings, final newline (`.editorconfig`).
- Conventional commits; `release-please-config.json` decides which types reach the changelog. The repo squash-merges with the PR title as the commit subject, so the title is the line that ships.

Full detail: `docs/CODING_STANDARDS.md`, plus `docs/go/CONVENTIONS.md`, `docs/go/OBJECTIVE_C.md`, and `docs/testing/TESTING_PATTERNS.md`.

## Agent Resources

- `.agents/skills/` is the canonical home for project skills (`create-pr`, `file-issue`); `.claude/skills` is a directory symlink to it — never add skill bodies there. Each skill may carry an `agents/openai.yaml` overlay for Codex.
- `.claude/settings.json` wires a non-blocking format-on-edit hook (`.claude/hooks/format-on-edit.sh`).
- `.cursor/rules/mimi.mdc` carries the same hard rules for Cursor.
- `.claude/worktrees/` is gitignored; the rest of `.claude/` is tracked.

## Documentation

Start here, then navigate: `docs/ARCHITECTURE.md` (shape) · `docs/DEVELOPMENT.md` · `docs/CLI.md` · `docs/CONFIGURATION.md` · `docs/TROUBLESHOOTING.md` · `docs/INSTALLATION.md`. Docs drift in places; when they disagree with code, read the code.

Keep this file lean — it loads into every agent session. Add only contracts an agent cannot infer from the code; workflow depth goes in a skill.
