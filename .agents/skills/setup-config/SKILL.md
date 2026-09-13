---
name: setup-config
description: "Set up or change a user's mimi config.toml: find or create the file, turn on hooks, borders, the systray, or the launchd service, then validate and apply it the way the running daemon needs. Use when a mimi user asks to configure mimi, write hooks, enable borders, or fix a config that does not take effect. For tiling layouts, use setup-layout."
---

# Setting up a mimi config

mimi reads one TOML file, and every key in it has a documented default. The
work is choosing what to turn on, then applying it correctly: some keys
reload on save, some need a daemon restart, and one needs the launchd
service reinstalled. The reference is `docs/CONFIGURATION.md` in the repo,
and `man mimi-config` on any install. This skill adds the order of
operations and the checks between steps.

## Where the reference lives

The user rarely has a checkout. A Homebrew install ships the app and man
pages only. Resolve the docs in this order:

1. A checkout in the working directory: `docs/CONFIGURATION.md` exists.
2. Man pages, installed by every method: `man mimi-config`,
   `man mimi-config-validate`, `man mimi-services-install`.
3. The docs at the installed version, fetched from GitHub:

   ```bash
   tag=$(mimi --version | sed -n '1s/^Mimi version //p')
   curl -fsSL "https://raw.githubusercontent.com/y3owk1n/mimi/$tag/docs/CONFIGURATION.md"
   ```

   A dev build prints `main-<sha>` or a `-dirty` suffix instead of a tag.
   Use `main` as the ref then.

The file `mimi config init` writes is fully commented and names every key,
so after step 2 below, the user's own file is the quickest reference.

## Steps

1. **Check the install.** `mimi status` reports whether the daemon runs,
   whether Accessibility is granted, and whether the socket is up. Window
   hooks, borders, and tiling all need Accessibility. Without it, they stay
   off and the daemon logs a warning rather than failing. Send the user to
   System Settings, Privacy & Security, Accessibility if it reads denied.

2. **Find or create the file.** The first existing path wins:
   `--config`, `$XDG_CONFIG_HOME/mimi/config.toml`,
   `~/.config/mimi/config.toml`, then `mimi.toml` in the working directory.
   When none exists, run `mimi config init`. Never run it over an existing
   file, it overwrites without asking. Read the existing file before
   editing it.

3. **Ask what they want, then edit only those sections.** The sections are
   `[settings]`, `[systray]`, `[tiling]`, `[border]`, and `[hooks]`. Leave
   the rest at defaults. A tiling request goes to the `setup-layout` skill.

4. **Validate.** `mimi config validate` must pass before anything else.
   Then `mimi config dump` prints the resolved config as JSON with defaults
   filled in, which is the check that a key landed where the user meant.
   Keys come out camelCase there, so `hook_timeout_secs` reads
   `hookTimeoutSecs`.

5. **Apply it the way the changed keys need.** Read the Reloading section
   of the reference and sort the keys the user changed:

   - Reloadable: `[hooks]`, `[tiling]`, `[border]`, and the hook and
     resize timing under `[settings]`. Saving the file is enough when the
     daemon runs. `mimi config reload` does the same on demand.
   - Restart-only: logging, worker count, pid and socket paths, and
     `[systray]`. Run `mimi services restart` for the installed service,
     or `mimi stop && mimi start` otherwise.
   - Reinstall-only: `settings.service_path`. Run `mimi services install`
     again. A restart does not apply it.

   The daemon logs which restart-only keys it could not apply on every
   reload until the file and the running daemon agree, so a change that
   "does nothing" is usually one of these.

6. **Prove it.** For a hook, fire the event and check the effect. For a
   border, focus a window. Do not declare done on a green validate alone.

## What the reference does not make obvious

- **Colours are alpha first.** `#rrggbb` or `#aarrggbb`. A `#rrggbbaa`
  value parses but draws the wrong thing.
- **Unknown hook keys are rejected.** `mimi config validate` fails on any
  `[hooks]` key outside the documented set, so a typo in a hook name is a
  validation error, not a silent no-op.
- **A bad config is rejected whole on reload.** The previous config stays
  in place and the failure is logged. Validate before saving so the user
  never runs on a stale config without knowing.
- **Hooks run under launchd's PATH, not the shell's.** A hook that works
  from a terminal and does nothing under the service needs
  `settings.service_path`, then `mimi services install`. It replaces the
  default PATH entirely and takes absolute directories only.
- **The CLI works without the daemon.** Actions fall back to direct
  execution when the socket is down, so a working `mimi action` proves
  nothing about the daemon. `mimi status` does.
- **Hook filters.** `app` and `bundle_id` are globs where `*` is the only
  wildcard, `title` is a regex, and `space` is a 1-based number that only
  workspace hooks honour. A leading `!` negates any of them, and `space`
  becomes a string when negated.
- **Never put user payloads in logs or examples you write for them.**
  Window titles and hook commands are private. mimi's own logging never
  records them, and hooks the skill writes should follow that.

## Installing the service

When the user wants mimi at login, `mimi services install` writes the
launchd plist and starts it. `mimi services status` confirms. Run install
again after changing `service_path`. Nix users configure the service in
their nix-darwin or home-manager module instead, and a hand-installed
service conflicts with that.
