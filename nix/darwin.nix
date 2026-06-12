{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.mimi;
  configFile =
    if cfg.configFile != null then cfg.configFile else pkgs.writeText "config.toml" cfg.config;
  effectiveEnv = {
    PATH = "/run/current-system/sw/bin:/nix/var/nix/profiles/default/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin";
  }
  // cfg.extraEnvironment;
in
{
  options = {
    services.mimi = {
      enable = lib.mkEnableOption "Mimi window and space utility daemon";
      package = lib.mkPackageOption pkgs "mimi" { };
      config = lib.mkOption {
        type = lib.types.lines;
        default = builtins.readFile ../configs/default-config.toml;
        description = "Config to use for {file} `mimi/config.toml`.";
      };
      configFile = lib.mkOption {
        type = lib.types.nullOr lib.types.path;
        default = null;
        description = "Path to existing config.toml configuration file. Takes precedence over config option.";
      };
      launchd = {
        enable = lib.mkEnableOption "the launchd agent managing the Mimi process" // {
          default = true;
        };
        keepAlive = lib.mkOption {
          type = lib.types.bool;
          default = true;
          description = "Whether the launchd service should be kept alive.";
        };
      };
      extraEnvironment = lib.mkOption {
        type = lib.types.attrsOf lib.types.str;
        default = { };
        example = {
          PATH = "/run/current-system/sw/bin:/nix/var/nix/profiles/default/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin";
        };
        description = ''
          Additional environment variables to set in the launchd service.
          These are merged with defaults such as a {env}`PATH`
          that includes common Nix binary directories.
          Setting {env}`PATH` here will override the default entirely.

          To extend the default PATH with additional directories:
          ```nix
          services.mimi.extraEnvironment = {
            PATH = "/Users/me/.local/bin:/run/current-system/sw/bin:/nix/var/nix/profiles/default/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin";
          };
          ```
        '';
      };
    };
  };
  config = (
    lib.mkIf (cfg.enable) {
      environment.systemPackages = [ cfg.package ];

      launchd.user.agents.mimi = {
        command =
          "${cfg.package}/Applications/Mimi.app/Contents/MacOS/mimi start"
          + (lib.optionalString (cfg.configFile != null || cfg.config != "") " --config ${configFile}");
        serviceConfig = {
          EnvironmentVariables = effectiveEnv;
          KeepAlive = cfg.launchd.keepAlive;
          RunAtLoad = true;
          StandardOutPath = "/tmp/mimi.log";
          StandardErrorPath = "/tmp/mimi.err.log";
          ProcessType = "Interactive";
          LimitLoadToSessionType = "Aqua";
          Nice = -10;
          ThrottleInterval = 10;
        };
      };
    }
  );
}
