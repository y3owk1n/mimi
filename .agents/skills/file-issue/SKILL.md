---
name: file-issue
description: "File a mimi bug report or feature request that matches the repo's issue forms: duplicate check first, every required field filled with real diagnostics, correct labels. Use when asked to open, file, or draft a GitHub issue for mimi. Not for pull requests."
---

# Filing an issue in mimi

Blank issues are disabled (`.github/ISSUE_TEMPLATE/config.yml`) — everything
goes through a form, and an agent-filed issue must contain the same fields the
form enforces. `gh issue create` bypasses form validation, so you are the
validator.

## Always, before filing

1. **Search for duplicates** — the forms make humans attest to this, so do it
   for real: `gh issue list --search "<keywords>" --state all`, plus
   discussions for feature ideas. Found one? Comment there instead of filing.
2. **Route non-issues away.** Questions, config help, and "here's my setup"
   belong in [Discussions](https://github.com/y3owk1n/mimi/discussions) — the
   forms link there deliberately. Filing an issue for these is wrong even if
   the user asked for an issue: say so and offer the right venue.
3. **Check whether it's macOS telling the truth.** Space switching and
   window-to-space moves ride private SkyLight APIs and synthetic dock swipes;
   a missing Accessibility grant looks exactly like a broken action. Confirm
   the permission state before calling it a bug — `docs/TROUBLESHOOTING.md` and
   `docs/ARCHITECTURE.md` (Permissions) cover which paths need which grant.

## Bug report (`--label bug`)

Mirror `bug_report.yml`'s fields as markdown sections, all of them:

- **Mimi version** — real output of `mimi version`, never guessed.
- **macOS version** — real output of `sw_vers`, quoted as e.g. `macOS 15.3.1`.
  Version matters more here than in most repos: the private APIs shift between
  releases.
- **What happened / What did you expect** — observed vs expected, concrete.
- **Steps to reproduce** — numbered, minimal, starting from a known state
  (`mimi daemon start`, or the exact `mimi action …` invocation).
- **Config (relevant sections)** — only the TOML sections involved, fenced as
  `toml`. Strip anything personal: hook commands run arbitrary shell, so
  scrub paths, hostnames, and command bodies that aren't load-bearing.
- **Screenshots / recordings** and **Additional context** when they help.

For daemon and hook bugs, the useful extras are: whether the daemon was
running (the CLI silently falls back to direct execution when the socket at
`settings.socket_file` is unavailable — a behavior difference that is itself
often the bug), which hook fired or didn't, and daemon logs at `debug` level.

## Feature request (`--label enhancement`)

Mirror `feature_request.yml`:

- **Problem or use case** — the need, not the mechanism. This is the section
  maintainers judge; write it first and best.
- **Proposed solution** — how it would work, with a concrete TOML snippet if
  it adds config, or the exact command line if it adds CLI surface.
- **Alternatives considered** — including whether an existing hook plus a
  shell command already gets there. mimi's hook system is the escape hatch;
  a request that shell already solves usually loses.
- **Screenshots / mockups**, optional.
- **Contribution** — state plainly whether a PR will follow.

## Filing

Draft the body, show it to the user before creating (an issue posts publicly
under their account), then:

```bash
gh issue create --title "<concise, user-facing summary>" --label bug --body-file <draft>
```

Title style matches the tracker: plain sentence, no conventional-commit
prefix, no trailing period.

The forms apply `bug` / `enhancement` automatically; `gh` does not, so pass it
yourself. `gh label list` is the authority on what else exists — today that's
`documentation`, `question`, `duplicate`, `invalid`, `good first issue`,
`help wanted`, plus the triage-state labels in `docs/agents/triage-labels.md`.
Don't invent new ones while filing.
