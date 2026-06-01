# Installation Guide

This guide covers installation methods for mimi, with the most complete support on macOS.

> [!NOTE]
> macOS is the only supported platform. mimi uses macOS-specific APIs (NSWorkspace, IOKit, CoreAudio, etc.) and requires CGo.

---

## Table of Contents

- [Requirements](#requirements)
- [Method 1: Homebrew (Recommended)](#method-1-homebrew-recommended)
- [Method 2: Nix Flake](#method-2-nix-flake)
- [Method 3: From Source](#method-3-from-source)
- [Post-Installation](#post-installation)
- [Troubleshooting](#troubleshooting)
- [Uninstallation](#uninstallation)

---

## Requirements

- macOS 14.0 or later
- Accessibility permission _(optional — only needed for window events)_

---

## Method 1: Homebrew (Recommended)

> [!NOTE]
> The homebrew tap is maintained in another repo: [y3owk1n/homebrew-tap](https://github.com/y3owk1n/homebrew-tap)
> If there's a problem with the tap, please open an issue in that repo or even better, a PR.

```bash
brew tap y3owk1n/tap

# Install latest stable release
brew install --cask y3owk1n/tap/mimi

# Upgrade to latest stable release
brew upgrade --cask y3owk1n/tap/mimi

# Uninstall stable
brew uninstall --cask y3owk1n/tap/mimi
```

---

## Method 2: Nix Flake

mimi is available as a Nix flake with built-in support for nix-darwin (macOS) and home-manager.

On macOS, `pkgs.mimi` uses the published release zip and `pkgs.mimi-source` builds from source.

### Add Flake Input

```nix
# flake.nix
{
  inputs = {
    mimi.url = "github:y3owk1n/mimi";
  };
}
```

### Option 1: nix-darwin Module (System-Level)

```nix
# flake.nix
{
  outputs = { self, nixpkgs, nix-darwin, mimi, ... }: {
    darwinConfigurations.your-hostname = nix-darwin.lib.darwinSystem {
      modules = [
        { nixpkgs.overlays = [ mimi.overlays.default ]; }
        mimi.darwinModules.default
        {
          services.mimi.enable = true;
          services.mimi.config = ''
            [settings]
            log_level = "info"

            [hooks]
            on_app_activate = ['echo "Focused: $mimi_APP_NAME"']
          '';
        }
      ];
    };
  };
}
```

**Module Options:**

- `services.mimi.enable` - Enable mimi (default: `false`)
- `services.mimi.package` - Package to use (default: `pkgs.mimi`) or `pkgs.mimi-source` for building from source
- `services.mimi.config` - Inline TOML configuration
- `services.mimi.configFile` - Path to existing config file (default: `null`, takes precedence over `config`)

The module automatically:

- Installs mimi system-wide
- Creates a launchd user agent with `KeepAlive = true` and `RunAtLoad = true`

### Option 2: home-manager Module (User-Level)

```nix
# flake.nix
{
  outputs = { self, nixpkgs, home-manager, mimi, ... }: {
    homeConfigurations.your-username = home-manager.lib.homeManagerConfiguration {
      pkgs = nixpkgs.legacyPackages.aarch64-darwin;
      modules = [
        { nixpkgs.overlays = [ mimi.overlays.default ]; }
        mimi.homeManagerModules.default
        {
          services.mimi.enable = true;
          services.mimi.config = ''
            [settings]
            log_level = "info"

            [hooks]
            on_app_activate = ['echo "Focused: $mimi_APP_NAME"']
          '';
        }
      ];
    };
  };
}
```

**Module Options:**

- `services.mimi.enable` - Enable mimi (default: `false`)
- `services.mimi.package` - Package to use (default: `pkgs.mimi`) or `pkgs.mimi-source` for building from source
- `services.mimi.config` - Inline TOML configuration
- `services.mimi.configFile` - Path to existing config file (default: `null`, takes precedence over `config`)
- `services.mimi.launchd.enable` - Enable the launchd agent (default: `true`)
- `services.mimi.launchd.keepAlive` - Keep the launchd service alive (default: `true`)

The module automatically:

- Installs mimi in user environment
- Creates `~/.config/mimi/config.toml` (or uses your `configFile`)
- Creates a launchd user agent with `KeepAlive` and `RunAtLoad = true`

### Using as an Overlay Only

```nix
{
  nixpkgs.overlays = [ mimi.overlays.default ];
  environment.systemPackages = [ pkgs.mimi ];
  # Or build from source:
  # environment.systemPackages = [ pkgs.mimi-source ];
}
```

### Updating

```bash
nix flake update mimi
# Then rebuild your system/home configuration
```

---

## Method 3: From Source

### Requirements

- Go 1.26+
- Xcode Command Line Tools
- Just command runner

### Build

```bash
git clone https://github.com/y3owk1n/mimi.git
cd mimi

# Development build
just build
mv ./bin/mimi /usr/local/bin/mimi

# Or build app bundle
just bundle
mv ./build/Mimi.app /Applications/Mimi.app
```

See [DEVELOPMENT.md](DEVELOPMENT.md) for detailed build options.

---

## Post-Installation

### 1. Grant Permissions

**Optional — Window events only:** Open System Settings → Privacy & Security → Accessibility → Add mimi (or your terminal emulator).

mimi will warn on startup if this permission is missing. All other events (app lifecycle, power, USB, network, clipboard, etc.) work without it.

### 2. Start mimi

```bash
# CLI
mimi start

# Or install as launchd service for auto-startup
mimi install
```

> [!NOTE]
> If mimi is already installed via nix-darwin or home-manager, `mimi install` will detect the conflict and refuse to install.

### 3. Verify

```bash
mimi status    # Should show "running" or "not running"
```

### 4. Configure

mimi loads config from `~/.config/mimi/config.toml`. See [CONFIGURATION.md](CONFIGURATION.md) for the full config reference.

---

## Troubleshooting

### "mimi wants to control this computer using accessibility features"

This is expected when window events are configured. Grant permission in System Settings → Privacy & Security → Accessibility.

### Command not found: mimi

If building from source, ensure the binary is in your PATH:

```bash
export PATH="/usr/local/bin:$PATH"
```

### Permission denied

```bash
chmod +x /usr/local/bin/mimi
```

### App won't open (macOS quarantine)

```bash
xattr -cr /Applications/Mimi.app
```

---

## Uninstallation

### Homebrew

```bash
brew uninstall --cask y3owk1n/tap/mimi
```

### Manual

```bash
# Stop and remove launchd agent
mimi uninstall

# Remove binary
rm /usr/local/bin/mimi

# Remove app bundle
rm -rf /Applications/Mimi.app

# Remove configuration and data
rm -rf ~/.config/mimi
rm -rf ~/.local/share/mimi

# Remove logs (if using log_file)
rm -f ~/.local/share/mimi/mimi.log*
```

### Nix

Remove the module from your configuration and rebuild.
