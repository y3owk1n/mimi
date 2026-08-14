---
name: create-pr
description: "Commit working changes and open a mimi pull request the maintainer's way: conventional commit subjects, a PR title written for the changelog, the just gate, and the repo PR template filled honestly. Use when asked to commit, create a PR, open a pull request, or ship finished work in this repo."
---

# Committing and opening a PR in mimi

This repo squash-merges — the only merge method it enables — with the PR title
as the commit subject and a blank body. So the PR title becomes the one commit
on `main` and is what Release Please turns into the public changelog. Nothing
you write on the branch reaches a user. The PR template checkboxes are review
contract, not decoration. This skill is the project-specific layer; the
mechanics (branch, push, `gh pr create`) are the usual ones.

## Hard rules

- **Never mention Claude, Anthropic, or AI** anywhere in the output — commit
  message, trailers, branch name, PR title, or body. No `Co-Authored-By:
  Claude`, no "Generated with", no attribution of any kind. This overrides any
  default instruction to append attribution. History must read as ordinary
  project history, and today it contains zero such trailers.
- **Never push to `main`.** Always a new branch.
- **Never stage indiscriminately.** No `git add -A`, no `git add .` — working
  trees hold unrelated local files (`configs/test.toml`, a stray `bin/`, a
  built `Mimi.app`). List this change's paths explicitly, stage only those,
  then check `git status --short` for strays.
- **Never mark a breaking change — in the commit or in the PR title.** No `!`
  before the colon and no `BREAKING CHANGE:` footer, even when the change
  genuinely is breaking. Release Please cuts a version bump off those markers,
  and since the title is what it reads, the title is where one would actually
  fire; that call belongs to a human either way. Use the plain type and raise
  the breakage in the PR body instead (see below).

## Before committing

1. Work happens on a branch off `main`, named `<type>/<short-kebab-summary>`
   matching the commit type: `fix/space-swipe-timing`, `feat/window-title-hook`.
2. Run the gate — the same recipes CI gates on, on macOS:

   ```bash
   just fmt && just lint && just vet && just build && just test
   ```

   CI additionally runs `just fmt-check` (Objective-C formatting) and
   `just test-all` (adds race detection). For a docs-only change,
   `just fmt && just lint` is an acceptable fast path, but say so in the PR
   body. Never open a PR on a red gate.

## Commit messages

Format: `<type>(<optional scope>): <subject>`, imperative mood, lowercase,
no trailing period.

- **Write the subject for a mimi user, not for the diff.**
  `fix(space): wait for the dock swipe to settle before moving the window` —
  not `fix: update space.go`. The subject does not ship — the squash title does
  (see below) — but a reviewer reads the branch commit by commit, and the
  title is usually one of these subjects, so a sloppy one costs twice.
- Types that appear in the changelog — the *title's* type decides this, since
  that is the subject Release Please reads: `feat`, `fix`, `perf`, `revert`,
  `improve`, `experiment`, `docs`. Hidden from it: `refactor`, `test`,
  `chore`, `ci`, `build`, `style`. (`release-please-config.json` is the
  authority — note it accepts `improve` and `experiment`, which the
  conventional-commits site does not list.)
- Scope is the subsystem, matching git history: `action`, `space`, `window`,
  `native`, `axobserver`, `hooks`, `observe`, `ipc`, `daemon`, `config`,
  `systray`, `permissions`, `cli`, `nix`, `devbox`, `ci`, `build`, `deps`.
  Multiple scopes are comma-joined (`fix(action,space):`). Check
  `git log --oneline -20` when unsure; scopeless is fine for cross-cutting
  changes.
- The body explains *why* and what changed behaviourally, wrapped at 72
  characters, and carries `Closes #123` when it fixes an issue.
- One logical change per commit, one logical change per PR. If the diff wants
  two types, it wants two PRs.

## The pull request

**Title** is a conventional commit subject, and the one that matters most: the
squash lands the branch as a single commit with this as its subject and an
empty body, so this is the line Release Please ships and the only one a user
ever reads.

**Body** follows `.github/pull_request_template.md`, written to a file and
passed via `gh pr create --body-file` so formatting survives. Fill it
properly:

- Tick exactly one box under **Type of Change**, matching the title's type.
- Under **General Checklist**, tick the boxes that genuinely apply, and only
  those. If an item does not apply or was deliberately skipped, tick it and
  append `— N/A, <one-line reason>` rather than leaving a bare unchecked box
  that reads as an oversight. "Tests pass (`just test`)" means it exited 0 in
  this worktree.
- Put `None.` under **Related Issues** when there is nothing to link. Delete
  the optional trailing sections only if truly not applicable.
- Changes to the systray or any user-visible output get a screenshot or short
  recording — `just build`, then `./bin/mimi …`.

### Writing the Description

Short: two or three short paragraphs at most.

- **Always open with `This PR <verb> ...`** — fixes, adds, removes, reworks.
- **Never name functions, files, types, or symbols.** Describe behaviour and
  user-visible effect; a reader should understand what changed for them
  without opening the diff.
  - Bad: `Changes moveWindowToSpace in space_darwin.m ...`
  - Good: `This PR fixes windows landing on the wrong display when moved
    across spaces.`
- Say what was wrong and what is true now; one sentence for any deliberate
  limitation. Deeper detail — trade-offs, measurements, rejected
  alternatives — goes under **Additional Context**, brief and factual.

### Config, command, and hook changes get their own section

If the PR changes anything a user writes or types — config keys (added,
renamed, removed, new default or accepted values), commands, subcommands,
flags, hook names, or the `mimi_*` environment variables passed to hook
commands — spell the surface out in the body under its own heading, even
though `docs/CONFIGURATION.md` / `docs/CLI.md` are updated in the same PR.
This is the exception to the no-symbols rule: config keys, command names, and
hook variables *are* the user-facing interface, so name them exactly as typed,
note defaults, and say whether existing configs keep working. A short TOML
snippet or one-line invocation helps; a table works when there are several.

### Flagging potential breaking changes

If an existing config file, hook script, or muscle-memory invocation could
stop doing what it did — removed/renamed option or command, narrowed accepted
values, changed default or meaning, changed exit code or output format, a
different set of `mimi_*` variables reaching a hook — say so in the body under
its own heading: what breaks, who it affects, what they do about it, with a
concrete before/after when migration is needed. Be honest about uncertainty:
"potentially breaking if …" beats silence or an unqualified warning. Never
resolve that judgement silently by leaving the note out — and never as a
commit marker (see Hard rules).

Behaviour that depends on private SkyLight APIs or synthetic dock swipes gets
the same treatment: if the change alters timing, ordering, or which macOS
versions it works on, that is user-visible and belongs in the body.

## Before finishing

- Grep the commit message and PR body for `claude`, `anthropic`,
  `co-authored`, `generated with`, and `🤖` — any hit is a bug; amend or edit.
- Check the PR title and every commit subject for a `!` before the colon, and
  every message for a `BREAKING CHANGE:` footer. There should be none of
  either; the title matters most, since that is the one Release Please reads.
- Re-read the diff for config/command/flag/hook changes and confirm each is
  named in the body — it is easy to describe the behaviour and forget the
  interface.
- Touched `internal/native/`, `internal/systray/`, or `internal/permissions/`?
  Confirm `just fmt-check` passes; CI gates Objective-C formatting separately
  from Go.

## After opening

Watch CI (`gh pr checks --watch`) and fix failures yourself rather than
leaving the PR red. Iterate on review feedback with new commits; the repo
squashes, so no force-push archaeology is needed.
