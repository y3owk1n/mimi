# Troubleshooting Guide

Common issues and solutions for mimi.

---

## Table of Contents

- [Quick Diagnosis](#quick-diagnosis)
- [Installation & Setup](#installation--setup)
- [Daemon Issues](#daemon-issues)
- [Permission Issues](#permission-issues)
- [Configuration Issues](#configuration-issues)
- [Hook Issues](#hook-issues)
- [Logging and Debugging](#logging-and-debugging)
- [Getting Help](#getting-help)
- [Emergency Reset](#emergency-reset)

---

## Quick Diagnosis

**Not working at all?** Check these first:

```bash
# 1. Is daemon running?
mimi status

# 2. Start daemon
mimi start

# 3. Check logs
mimi status                    # Shows status
mimi events                    # Live event stream
```

**Common issues:**

- ❌ **"mimi: not running"** → Start the daemon with `mimi start`
- ❌ **Window events not firing** → Grant accessibility permissions
- ❌ **Hooks not executing** → Check hook syntax with `mimi config validate` and test with `mimi test <event-kind>`

---

## Installation & Setup

**"Command not found: mimi"**

```bash
# Add to PATH
export PATH="/usr/local/bin:$PATH"
# Add to ~/.zshrc or ~/.bashrc
```

**"Cannot open Mimi.app because the developer cannot be verified"**

```bash
xattr -cr /Applications/Mimi.app  # Remove quarantine
```

**Homebrew fails**

```bash
brew update && brew reinstall --cask mimi
```

---

## Daemon Issues

### Daemon won't start

**Check config first:**

```bash
mimi config validate          # Validate config syntax
```

**Check logs (if configured with `log_file`):**

```bash
cat ~/.local/share/mimi/mimi.log
```

**Try with fresh config:**

```bash
mimi config init              # Overwrite with defaults
mimi start
```

### Daemon stops responding

```bash
# Force quit
pkill -9 mimi

# Restart
mimi start

# Monitor logs (if log_file is configured)
tail -f ~/.local/share/mimi/mimi.log
```

### Daemon won't quit

```bash
pkill -9 mimi
```

Or use Activity Monitor → search "mimi" → Force Quit.

---

## Permission Issues

### Accessibility Permission

Window events (`window_focus`, `window_title_change`, `window_resize`, etc.) require Accessibility permission.

**Grant permissions:**

System Settings → Privacy & Security → Accessibility → Add mimi (or your terminal emulator)

**Check if granted:**

```bash
mimi status
# Look for accessibility warning on startup
```

**Reset if not working:**

1. Remove mimi from the accessibility list
2. Re-add mimi
3. Restart daemon: `pkill -9 mimi && mimi start`

---

## Configuration Issues

### Config changes not taking effect

**Daemon needs restart:**

```bash
mimi stop && mimi start
```

Or send SIGHUP if the daemon supports hot-reload:

```bash
kill -HUP $(pgrep mimi)
```

### "Failed to parse config"

**TOML syntax error.**

```bash
mimi config validate          # Shows parse errors
```

Common issues:

- Missing quotes around string values
- Incorrect section headers
- Invalid TOML syntax

### Hooks not matching events

Check your hook filters:

```toml
[hooks]
# This hook has an app filter — only fires for Slack
on_app_activate = [
  { run = "echo 'hello'", app = "Slack" }
]

# This hook has no filter — fires for all apps
on_app_activate = [
  "echo 'hello'"
]
```

Test with a specific event kind:

```bash
mimi test app_activate --app Slack
```

---

## Hook Issues

### Hook command not running

**Check:**

1. Is the hook key correct? See [CONFIGURATION.md](CONFIGURATION.md#event-reference) for all event keys.
2. Does the hook match the event filters? Check `app`, `bundle_id`, `title` filters.
3. Test directly:

```bash
mimi test app_activate --app "Slack"
```

### Hook times out

**Increase timeout:**

```toml
[hooks]
on_app_activate = [
  { run = "~/scripts/long-running.sh", timeout_secs = 60 }
]
```

Or run asynchronously:

```toml
on_app_activate = [
  { run = "~/scripts/long-running.sh", async = true }
]
```

### Hook runs too many times

**Add filters:**

```toml
[hooks]
# Only fire for Slack
on_app_activate = [
  { run = "my-script.sh", app = "Slack" }
]
```

### Hook output not visible

Hooks run in the background with no stdout/stderr capture. Redirect output to a file:

```toml
[hooks]
on_app_activate = [
  'echo "Focused: $mimi_APP_NAME" >> ~/mimi-hooks.log'
]
```

---

## Logging and Debugging

### Enable debug logging

```toml
[settings]
log_level = "debug"
```

Restart:

```bash
mimi stop && mimi start
```

### View logs

```bash
# If log_file is configured:
tail -f ~/.local/share/mimi/mimi.log

# Last 100 lines
tail -100 ~/.local/share/mimi/mimi.log

# Search for errors
grep error ~/.local/share/mimi/mimi.log

# Live event stream
mimi events
```

### Event stream

```bash
# Watch events in real-time
mimi events

# Filter by event kind
mimi events --kind app_activate

# Filter by app
mimi events --app Slack

# Raw JSON lines
mimi events --json
```

### Common log messages

**"accessibility permission not granted — window events disabled"** — Window events won't work. Grant accessibility access in System Settings.

**"config reloaded"** — Config file was hot-reloaded successfully.

**"hook ok"** — Hook command completed successfully.

**"hook failed"** — Hook command exited with non-zero status.

### Clear logs

```bash
rm ~/.local/share/mimi/mimi.log
mimi start  # Creates fresh log
```

---

## Getting Help

If none of these solutions work:

1. **Gather information:**
   - macOS version: `sw_vers`
   - mimi version: `mimi --version`
   - Config file (anonymize if needed)
   - Relevant logs: `tail -50 ~/.local/share/mimi/mimi.log` (if configured)
   - Recent events: `mimi status`

2. **Search existing issues:**
   - <https://github.com/y3owk1n/mimi/issues>

3. **Open an issue:**
   - Include all gathered information
   - Describe expected vs actual behaviour
   - Steps to reproduce
   - Config file (anonymized)

4. **Consider a PR:**
   - Pull requests are welcome
   - See [DEVELOPMENT.md](DEVELOPMENT.md) for contribution guidelines

---

## Emergency Reset

If mimi is completely broken:

```bash
# 1. Force quit
pkill -9 mimi

# 2. Remove all mimi files
rm -f /usr/local/bin/mimi
rm -rf /Applications/Mimi.app
rm -rf ~/.config/mimi
rm -rf ~/.local/share/mimi
rm -f ~/Library/LaunchAgents/com.y3owk1n.mimi.plist

# 3. Reinstall
brew reinstall --cask y3owk1n/tap/mimi
# or build from source

# 4. Fresh start
mimi start
```
