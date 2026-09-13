# Installation guide

This guide covers the ways to install Mimi on macOS.

---

## Table of contents

- [Requirements](#requirements)
- [Method 1: Homebrew (recommended)](#method-1-homebrew-recommended)
- [Method 2: Nix flake](#method-2-nix-flake)
- [Method 3: From source](#method-3-from-source)
- [Post-installation](#post-installation)
- [Shell completions](#shell-completions)
- [Troubleshooting](#troubleshooting)
- [Uninstallation](#uninstallation)

---

## Requirements

- macOS 14.0 or later
- Accessibility permission (granted during setup)

---

## Method 1: Homebrew (recommended)

> [!NOTE]
> The Homebrew tap lives in a separate repo, [y3owk1n/homebrew-tap](https://github.com/y3owk1n/homebrew-tap).
> Report problems with the tap in that repo, or open a PR there.

```bash
brew tap y3owk1n/tap
brew install --cask y3owk1n/tap/mimi
```

---

## Method 2: Nix flake

The flake exports an overlay, a nix-darwin module, and a home-manager module.

> `pkgs.mimi` uses the published release zip and `pkgs.mimi-source` builds from source.

### Add the flake input

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

### Option 1: nix-darwin module (system-level)

Use the nix-darwin module for a system-wide installation:

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

Module options:

- `services.mimi.enable` - Enable mimi (default: `false`)
- `services.mimi.package` - Package to use (default: `pkgs.mimi`, the release zip). Set `pkgs.mimi-source` to build from source.
- `services.mimi.config` - Inline TOML configuration (default: the contents of `configs/default-config.toml`)
- `services.mimi.configFile` - Path to an existing config file (default: `null`, takes precedence over `config`)
- `services.mimi.launchd.enable` - Enable the launchd agent (default: `true`)
- `services.mimi.launchd.keepAlive` - Sets the agent's `KeepAlive` (default: `true`)
- `services.mimi.extraEnvironment` - Extra environment variables for the launchd agent (default: `{}`). They merge over a default `PATH` of `/run/current-system/sw/bin:/nix/var/nix/profiles/default/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin`. Setting `PATH` here replaces that default.

When enabled, the module:

- Adds the package to `environment.systemPackages`
- Creates a launchd user agent, when `launchd.enable` is set, with the configured environment, `RunAtLoad = true`, and `KeepAlive` set from `launchd.keepAlive`
- Passes the config to the agent with `--config`, as a file in the Nix store

Both packages install shell completions for bash, fish, and zsh.

> [!NOTE]
> **Codesign for source builds (`mimi-source`):** The Go linker signs the binary,
> but that signature has no hardened runtime entitlements. To embed
> `Mimi.entitlements` with `--options runtime`, run Apple's `codesign` outside
> the build sandbox. The package bundles the entitlements file at
> `Contents/Resources/Mimi.entitlements`.
>
> The default `pkgs.mimi` (zip) package does not need this. Its app bundle is already signed with the entitlements.
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

### Option 2: home-manager module (user-level)

Use the home-manager module for a per-user installation:

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

Module options:

- `services.mimi.enable` - Enable mimi (default: `false`)
- `services.mimi.package` - Package to use (default: `pkgs.mimi`, the release zip). Set `pkgs.mimi-source` to build from source.
- `services.mimi.config` - Inline TOML configuration (default: the contents of `configs/default-config.toml`)
- `services.mimi.configFile` - Path to an existing config file (default: `null`, takes precedence over `config`)
- `services.mimi.launchd.enable` - Enable the launchd agent (default: `true`)
- `services.mimi.launchd.keepAlive` - Sets the agent's `KeepAlive` (default: `true`)
- `services.mimi.extraEnvironment` - Extra environment variables for the launchd agent (default: `{}`). They merge over a default `PATH` that starts with `~/.nix-profile/bin` and `/etc/profiles/per-user/<username>/bin`, then the system Nix directories, `/usr/local/bin:/usr/bin:/bin`, and `/opt/homebrew/bin`. Setting `PATH` here replaces that default.

When enabled, the module:

- Adds the package to `home.packages`
- Writes `~/.config/mimi/config.toml` from `config`, or links it to your `configFile`
- Creates a launchd user agent (when `launchd.enable` is `true`) with `KeepAlive`, `RunAtLoad = true`, and the configured environment. The agent starts mimi with `--config` pointing at `~/.config/mimi/config.toml`.

> [!NOTE]
> **Codesign for source builds (`mimi-source`):** The Go linker signs the binary,
> but that signature has no hardened runtime entitlements. To embed
> `Mimi.entitlements` with `--options runtime`, run Apple's `codesign` outside
> the build sandbox. The package bundles the entitlements file at
> `Contents/Resources/Mimi.entitlements`.
>
> The default `pkgs.mimi` (zip) package does not need this. Its app bundle is already signed with the entitlements.
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

### Option 3: Overlay or package only

To manage the service yourself, use only the overlay:

> [!NOTE]
> Without a module you write the config and the launch agent yourself.

```nix
{
  outputs = { self, nixpkgs, nix-darwin, mimi, ... }: {
     darwinConfigurations.your-hostname = nix-darwin.lib.darwinSystem {
       modules = [
         ({ pkgs, ... }: {
           nixpkgs.overlays = [ mimi.overlays.default ];
           environment.systemPackages = [ pkgs.mimi ];
         })
       ];
     };
  };
}
```

Or install the flake's package directly:

```nix
{
  outputs = { self, nixpkgs, nix-darwin, mimi, ... }: {
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
  home.packages = [ mimi.packages.${system}.default ];
}
```

The flake's `packages` output has `default` (the release zip) and `source` (built from source), for `aarch64-darwin` only.

### Configuration examples

Minimal setup (nix-darwin):

```nix
{
  services.mimi.enable = true;
}
```

Inline config (home-manager):

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

External config file (home-manager):

```nix
{
  services.mimi.enable = true;
  services.mimi.configFile = ./dotfiles/mimi/config.toml;
}
```

### Updating

To update Mimi, update the flake lock:

```bash
nix flake update mimi
# Then rebuild your system/home configuration
```

---

### Tiling on Nix

Both packages ship the example layouts at
`${pkgs.mimi}/share/mimi/examples/tiling` (the same path in `pkgs.mimi-source`).
They are Python scripts that use only the standard library, so the daemon needs
a `python3` it can find. The default `PATH` of each module includes the
directories its packages land in: your Home Manager profile for home-manager,
and `/run/current-system/sw/bin` for nix-darwin. Adding `pkgs.python3` to your
packages is enough. You can run the layouts in two ways.

Run them from the store. You copy nothing, and a new mimi brings new examples.
Name the interpreter and the layout by store path so that neither depends on
`PATH`:

```nix
{ pkgs, ... }:
{
  services.mimi = {
    enable = true;
    config = ''
      [tiling]
      enabled = true
      layout = "${pkgs.python3}/bin/python3 ${pkgs.mimi}/share/mimi/examples/tiling/bsp.py"
      relayout_on_drag = true
    '';
  };
}
```

Keep your own copy. Copy `examples/tiling` into your dotfiles, edit it, and let
Home Manager put the directory in place. The layouts import `rules.py` from
their own directory, so ship the whole directory:

```nix
{ config, pkgs, ... }:
{
  home.packages = [ pkgs.python3 ];

  xdg.configFile."mimi/tiling" = {
    source = ./tiling;      # your copy of examples/tiling
    recursive = true;
  };

  services.mimi = {
    enable = true;
    config = ''
      [tiling]
      enabled = true
      layout = "${config.xdg.configHome}/mimi/tiling/bsp.py"
      relayout_on_drag = true
    '';
  };
}
```

With nix-darwin, put the layouts wherever you keep per-user files and name them
in the config the same way. With either module, `mimi tiling preview` prints
what the layout would do before the daemon applies anything. Hotkeys call the
same `mimi tiling cmd ...` commands as on any other install. See the
[Tiling Guide](TILING.md).

---

## Method 3: From source

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

See [DEVELOPMENT.md](DEVELOPMENT.md) for more build options.

---

## Post-installation

### 1. Grant permissions

Required: open System Settings > Privacy & Security > Accessibility and add Mimi.

If you only use the CLI, you are done. To run the daemon, continue with the steps below.

### 2. Start Mimi (optional)

```bash
# CLI, or the binary inside the app bundle
mimi start
/Applications/Mimi.app/Contents/MacOS/mimi start

# Or install as a launchd service that starts at login
mimi services install
```

If Accessibility is not granted yet, `mimi start` shows an "Accessibility Permission Needed" alert. If no config file exists, it offers to create one at the default path.

> [!NOTE]
> `services install` checks one launchd label, Mimi's own `com.y3owk1n.mimi`. It
> refuses when that label is already loaded with no plist of Mimi's behind it,
> and when `~/Library/LaunchAgents/com.y3owk1n.mimi.plist` exists but is not a
> regular file (a symlink, for example).
>
> It cannot see an installation registered under any other label. The
> nix-darwin and home-manager modules above register agents under their own
> labels. If you installed Mimi that way, check before you run it.
> `launchctl list | grep -i mimi` lists every agent launchd holds whose label
> contains "mimi", and both module labels do. If one is already there, keep it
> and skip `services install`, so that you do not end up with two daemons.

### 3. Verify (optional)

```bash
mimi --version
mimi status  # Should show "mimi: running (pid ...)"
```

### 4. Configure (optional)

Mimi loads config from `~/.config/mimi/config.toml` by default. See [CONFIGURATION.md](CONFIGURATION.md) for the full search order.

To get started, run `mimi config init`, which writes the default config to that path and overwrites any existing file. You can also copy `configs/default-config.toml` there by hand.

See [CONFIGURATION.md](CONFIGURATION.md) for all options, and [TROUBLESHOOTING.md](TROUBLESHOOTING.md) if something does not work.

### 5. Agent skills (optional)

The repo carries three skills for coding agents such as Claude Code, Codex, and Cursor: `ask-mimi` answers what mimi can do and which command or skill to use, `setup-config` walks through the config file and applies it correctly, and `setup-layout` gets tiling working with one of the example layouts or a custom one. All three fetch the docs and example layouts they need at the installed version, so a Homebrew install needs no checkout.

Install them into the current project with the [skills](https://skills.sh) CLI:

```bash
npx skills add y3owk1n/mimi
```

Add `-g` to install them for every project. A checkout of the repo has them already under `.agents/skills/`.

---

## Shell completions

`mimi completion` generates completion scripts for bash, zsh, and fish. The Nix packages install them already.

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

### "Accessibility Permission Needed" alert

`mimi start` shows this alert while Mimi lacks Accessibility permission. Click Request Permission to open the macOS prompt, grant access in System Settings, then click Granted, Start Mimi. If macOS has not applied the grant to the running process yet, Mimi shows "Mimi Restart Required" and quits. Run `mimi start` again.

### Command not found: mimi

If you built the CLI, make sure the directory you moved it to is on your `PATH`:

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

The release app is ad-hoc signed, not notarized, so macOS may quarantine it. Clear the quarantine attribute:

```bash
xattr -cr /Applications/Mimi.app
```

Then try again.

### Nix build fails on an Intel Mac

The flake's `packages` output covers `aarch64-darwin` only. On an Intel Mac, apply `mimi.overlays.default` and use `pkgs.mimi`. The package picks the `mimi-darwin-amd64.zip` release asset for `x86_64-darwin` on its own.

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

# Remove PID file and socket (default pid_file and socket_file)
rm -rf ~/.local/share/mimi

# Remove the service's captured console output (when log_file is unset)
rm -f /tmp/mimi.log /tmp/mimi.err.log
```

If you set `settings.log_file`, also remove that file and its `.out.log` and `.err.log` siblings.

### Nix

Remove the module from your configuration and rebuild.
