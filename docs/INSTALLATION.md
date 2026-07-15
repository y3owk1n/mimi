# Installation Guide

This guide covers installation methods for Mimi on macOS.

---

## Table of Contents

- [Requirements](#requirements)
- [Method 1: Homebrew (Recommended)](#method-1-homebrew-recommended)
- [Method 2: Nix Flake](#method-2-nix-flake)
- [Method 3: From Source](#method-3-from-source)
- [Post-Installation](#post-installation)
- [Shell Completions](#shell-completions)
- [Troubleshooting](#troubleshooting)
- [Uninstallation](#uninstallation)

---

## Requirements

- macOS 14.0 or later
- Accessibility permissions (granted during setup)

---

## Method 1: Homebrew (Recommended)

> [!NOTE]
> The homebrew tap is maintained in another repo: [y3owk1n/homebrew-tap](https://github.com/y3owk1n/homebrew-tap)
> If there's a problem with the tap, please open an issue in that repo or even better, a PR.

```bash
brew tap y3owk1n/tap
brew install --cask y3owk1n/tap/mimi
```

---

## Method 2: Nix Flake

Mimi is available as a Nix flake with built-in support for nix-darwin and home-manager.

> `pkgs.mimi` uses the published release zip and `pkgs.mimi-source` builds from source.

### Add Flake Input

Add Mimi to your flake inputs:

```nix
# flake.nix
{
  inputs = {
     # ... other inputs
     mimi.url = "github:y3owk1n/mimi"; # or "https://flakehub.com/f/y3owk1n/mimi/0.1"
     # ... other inputs
  };
}
```

### Option 1: nix-darwin Module (System-Level)

Use the nix-darwin module for system-wide installation:

```nix
# flake.nix
{
  outputs = { self, nixpkgs, nix-darwin, mimi, ... }: {
     darwinConfigurations.your-hostname = nix-darwin.lib.darwinSystem {
       modules = [
         # Apply the Mimi overlay
         {
           nixpkgs.overlays = [ mimi.overlays.default ];
         }

         # Import the Mimi module
         mimi.darwinModules.default

         # Configure Mimi
         {
            # Enable Mimi
            services.mimi.enable = true;

            # Optional: Use specific package version
            # services.mimi.package = pkgs.mimi; # This will use the latest version
            # services.mimi.package = pkgs.mimi-source; # This will build from source

            # Optional: Inline configuration
            services.mimi.config = ''
				[settings]
				hook_shell = "/bin/dash"

				[systray]
				enabled = true
				show_workspace_number = true
            '';
         }
       ];
     };
  };
}
```

**Module Options:**

- `services.mimi.enable` - Enable mimi (default: `false`)
- `services.mimi.package` - Package to use (default: `pkgs.mimi` for latest version) or `pkgs.mimi-source` for building from source
- `services.mimi.config` - Inline TOML configuration (default: uses `configs/default-config.toml`)
- `services.mimi.configFile` - Path to existing config file (default: `null`, takes precedence over `config`)
- `services.mimi.launchd.enable` - Enable the launchd agent (default: `true`)
- `services.mimi.launchd.keepAlive` - Keep the launchd service alive (default: `true`)
- `services.mimi.extraEnvironment` - Additional environment variables for the launchd service (default: `{}`; includes a sensible `PATH` with Nix binary directories)

The module automatically:

- Installs Mimi system-wide
- Creates a launchd user agent with the configured environment
- Configures the agent to run at login with `KeepAlive` and `RunAtLoad = true`
- Installs shell completions for bash, fish, and zsh

> [!NOTE]
> **Codesign for source builds (`mimi-source`):** The Go linker signs the binary
> automatically, but this linker signature lacks hardened runtime entitlements.
> To embed our `Mimi.entitlements` with `--options runtime`, use Apple's `codesign`
> (available outside the build sandbox). The entitlements file is bundled at
> `Contents/Resources/Mimi.entitlements`.
>
> This is not needed for the default `pkgs.mimi` (zip) package, which is pre-signed.
>
> #### nix-darwin
>
> ```nix
> { config, lib, ... }:
>
> let
>   appPath = "/Applications/Nix Apps/Mimi.app";
>   entitlements = "${appPath}/Contents/Resources/Mimi.entitlements";
> in {
>   system.activationScripts.postActivation.text = ''
>     if [ -e "${appPath}" ]; then
>       echo "Codesigning Mimi.app..."
>       /usr/bin/codesign --force --sign - \
>         --entitlements "${entitlements}" \
>         --options runtime \
>         --timestamp=none \
>         "${appPath}"
>     fi
>   '';
> }
> ```

### Option 2: home-manager Module (User-Level)

Use the home-manager module for user-specific installation on macOS or Linux:

```nix
# flake.nix
{
  outputs = { self, nixpkgs, home-manager, mimi, ... }: {
     homeConfigurations.your-username = home-manager.lib.homeManagerConfiguration {
       pkgs = nixpkgs.legacyPackages.aarch64-darwin;

       modules = [
         # Apply the Mimi overlay
         {
           nixpkgs.overlays = [ mimi.overlays.default ];
         }

         # Import the Mimi module
         mimi.homeManagerModules.default

         # Configure Mimi
         {
           # Enable Mimi
           services.mimi.enable = true;

           # Optional: Use specific package version
           # services.mimi.package = pkgs.mimi; # This will use the latest version
           # services.mimi.package = pkgs.mimi-source; # This will build from source

           # Option A: Inline configuration
           services.mimi.config = ''
				[settings]
				hook_shell = "/bin/dash"

				[systray]
				enabled = true
				show_workspace_number = true
           '';

           # Option B: Use existing config file (takes precedence)
           # services.mimi.configFile = ./path/to/config.toml;
         }
       ];
     };
  };
}
```

**Module Options:**

- `services.mimi.enable` - Enable mimi (default: `false`)
- `services.mimi.package` - Package to use (default: `pkgs.mimi`; uses release artifact on Linux, builds from source if unavailable)
- `services.mimi.config` - Inline TOML configuration (default: uses `configs/default-config.toml`)
- `services.mimi.configFile` - Path to existing config file (default: `null`, takes precedence over `config`)
- `services.mimi.launchd.enable` - Enable the launchd agent on macOS (default: `true`)
- `services.mimi.launchd.keepAlive` - Keep the launchd service alive on macOS (default: `true`)
- `services.mimi.extraEnvironment` - Additional environment variables for the launchd or systemd service (default: `{}`; includes a sensible `PATH` with Nix binary directories and the user's Nix profile)

The module automatically:

- Installs Mimi in user environment
- Creates `~/.config/mimi/config.toml` (or uses your `configFile`)
- **macOS:** Creates a launchd user agent (if `launchd.enable` is `true`) with `KeepAlive`, `RunAtLoad = true`, and the configured environment
- Installs shell completions for bash, fish, and zsh

> [!NOTE]
> **Codesign for source builds (`mimi-source`):** The Go linker signs the binary
> automatically, but this linker signature lacks hardened runtime entitlements.
> To embed our `Mimi.entitlements` with `--options runtime`, use Apple's `codesign`
> (available outside the build sandbox). The entitlements file is bundled at
> `Contents/Resources/Mimi.entitlements`.
>
> This is not needed for the default `pkgs.mimi` (zip) package, which is pre-signed.
>
> #### Home Manager
>
> ```nix
> { config, lib, ... }:
>
> let
>   username = config.home.username or "changeme";
>   appPath = "/Users/${username}/Applications/Home Manager Apps/Mimi.app";
>   entitlements = "${appPath}/Contents/Resources/Mimi.entitlements";
> in {
>   home.activation.signMimi = lib.hm.dag.entryAfter [ "copyApps" ] ''
>     if [ -e "${appPath}" ]; then
>       echo "Codesigning Mimi.app..."
>       /usr/bin/codesign --force --sign - \
>         --entitlements "${entitlements}" \
>         --options runtime \
>         --timestamp=none \
>         "${appPath}"
>     fi
>   '';
> }
> ```

### Option 3: Using as an Overlay Only

If you prefer to manage the service yourself, you can just use the overlay:

> [!NOTE]
> Direct installation requires manual configuration and launch agent setup.

```nix
{
  outputs = { self, nixpkgs, mimi, ... }: {
     darwinConfigurations.your-hostname = nix-darwin.lib.darwinSystem {
       modules = [
         {
           nixpkgs.overlays = [ mimi.overlays.default ];
           environment.systemPackages = [ pkgs.mimi ];
         }
       ];
     };
  };
}
```

Or install directly as a package:

```nix
{
  outputs = { self, nixpkgs, mimi, ... }: {
     darwinConfigurations.your-hostname = nix-darwin.lib.darwinSystem {
       modules = [
         {
           environment.systemPackages = [
             mimi.packages.aarch64-darwin.default
           ];
         }
       ];
     };
  };
}
```

Or with home-manager:

```nix
{
  home.packages = [ mimi.packages.${system}.mimi ];
}
```

### Configuration Examples

**Minimal setup (nix-darwin):**

```nix
{
  services.mimi.enable = true;
}
```

**Custom hotkeys (home-manager):**

```nix
{
  services.mimi.enable = true;
  services.mimi.config = ''
	[settings]
	hook_shell = "/bin/dash"

	[systray]
	enabled = true
	show_workspace_number = true
  '';
}
```

**Using external config file (home-manager):**

```nix
{
  services.mimi.enable = true;
  services.mimi.configFile = ./dotfiles/mimi/config.toml;
}
```

### Updating

To update Mimi, update your flake lock:

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

# Build CLI
just release
mv ./bin/mimi /usr/local/bin/mimi

# Or build app bundle
just bundle
mv ./build/Mimi.app /Applications/Mimi.app
```

See [DEVELOPMENT.md](DEVELOPMENT.md) for detailed build options.

---

## Post-Installation

### 1. Grant Permissions

**Required:** Open System Settings → Privacy & Security → Accessibility → Add Mimi

You are done if you just want to use it as CLI. If you want the full experience with daemon, continue the following steps.

### 2. Start Mimi (OPTIONAL)

```bash
# App bundle
open -a Mimi

# Or CLI
mimi start

# Or install as launchd service for auto-startup
mimi services install
```

> [!NOTE]
> If Mimi is already installed via nix-darwin, home-manager, or other methods, `services install` will detect the conflict and refuse to install. Check your existing configurations first.

### 3. Verify (OPTIONAL)

```bash
mimi --version
mimi status  # Should show "running"
```

### 4. Configure (OPTIONAL)

Mimi loads config from `~/.config/mimi/config.toml` (recommended). See [CONFIGURATION.md](CONFIGURATION.md) for the full search order.

**Get started:** Copy `configs/default-config.toml` to `~/.config/mimi/config.toml`

See [CONFIGURATION.md](CONFIGURATION.md) for all options. Having issues? Check [TROUBLESHOOTING.md](TROUBLESHOOTING.md).

---

## Shell Completions

Mimi provides shell completions for bash, zsh, and fish.

### Bash

```bash
mimi completion bash > /usr/local/etc/bash_completion.d/mimi
```

### Zsh

```bash
mimi completion zsh > "${fpath[1]}/_mimi"
```

### Fish

```bash
mimi completion fish > ~/.config/fish/completions/mimi.fish
```

---

## Troubleshooting

### "Mimi wants to control this computer using accessibility features"

This is normal. Click **OK** and grant permissions in System Settings.

### Command not found: mimi

If using the CLI build, ensure the binary is in your PATH:

```bash
# Add to ~/.zshrc or ~/.bashrc
export PATH="/usr/local/bin:$PATH"
```

### Permission denied

Make the binary executable:

```bash
chmod +x /usr/local/bin/mimi
```

### App won't open (macOS quarantine)

macOS may quarantine apps from unidentified developers:

```bash
xattr -cr /Applications/Mimi.app
```

Then try opening again.

### Nix build fails

Ensure you're on an Apple Silicon Mac (arm64). For Intel Macs, change the URL to:

```nix
url = "https://github.com/y3owk1n/mimi/releases/download/v${version}/mimi-darwin-amd64.zip";
```

---

## Uninstallation

### Homebrew

```bash
brew uninstall --cask y3owk1n/tap/mimi
```

### Manual

```bash
# Stop and remove launchd service (if installed)
mimi services uninstall

# Remove app bundle
rm -rf /Applications/Mimi.app

# Remove CLI
rm /usr/local/bin/mimi

# Remove configuration
rm -rf ~/.config/mimi
rm -rf ~/Library/Application\ Support/mimi

# Remove logs
rm -rf ~/Library/Logs/mimi
```

### Nix

Remove the module from your configuration and rebuild.
