# Installation

## Homebrew

```bash
brew tap y3owk1n/tap
brew install --cask y3owk1n/tap/mimi
```

## From Source

```bash
git clone https://github.com/y3owk1n/mimi.git
cd mimi
just build
# binary at bin/mimi
```

## Nix

```nix
{
  inputs.mimi.url = "github:y3owk1n/mimi";

  outputs = { self, mimi, ... }: {
    nixosConfigurations.myhost = nixpkgs.lib.nixosSystem {
      modules = [ mimi.nixosModules.home-mimi ];
    };
  };
}
```

Enable with `services.mimi.enable = true` in Home Manager or nix-darwin.

---

## Permissions

Grant **Accessibility** to mimi in:

**System Settings → Privacy & Security → Accessibility**

Required for:

- `mimi action` commands
- Window hooks (`on_window_*`)

---

## Auto-start (launchd)

```bash
mimi config init
mimi services install
mimi services start
```

This installs `~/Library/LaunchAgents/com.y3owk1n.mimi.plist` and runs `mimi start` at login.

Verify:

```bash
mimi services status
launchctl list | grep mimi
```

---

## Integration with Hotkey Tools

Bind `mimi action` commands in skhd, Karabiner, Raycast, or Hammerspoon:

```bash
# skhd example
alt - 1 : mimi action space 1
alt - tab : mimi action focus_window
```

No daemon required for action commands.
