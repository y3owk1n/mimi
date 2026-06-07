# Troubleshooting

## `mimi action space` or `move_window_to_space` does nothing

1. **Rebuild** after updates — native run-loop pumping is required for CLI actions
2. **Grant Accessibility** to the exact binary you run (`bin/mimi` or `Mimi.app`)
3. **Check space index** — spaces are 1-based in Mission Control order (`mimi action space 1` is the first space)
4. **Close Mission Control** — actions refuse to run while Mission Control is open

## Window hooks not firing

1. Run `mimi status` — confirm daemon is running and Accessibility is granted
2. Run `mimi config validate` — confirm hooks are defined
3. Set `log_level = "debug"` in config and check logs
4. Window hooks require Accessibility; workspace hooks do not

## Daemon won't start

```bash
mimi config validate
mimi status          # check for stale PID file
rm ~/.local/share/mimi/mimi.pid
mimi start
```

## launchd service issues

```bash
mimi services status
launchctl list | grep mimi
cat /tmp/mimi.err.log    # if using Nix module
```

## Permission prompt keeps appearing

Remove and re-add mimi in System Settings → Privacy & Security → Accessibility. Ensure you're granting the binary you actually execute (Homebrew cask path vs local `bin/mimi`).
