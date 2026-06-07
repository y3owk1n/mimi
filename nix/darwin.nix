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
in
{
  options = {
    services.mimi = {
      enable = lib.mkEnableOption "Mimi window and space utility daemon";
      package = lib.mkPackageOption pkgs "mimi" { };
      config = lib.mkOption {
        type = lib.types.lines;
        default = builtins.readFile ./configs/default-config.toml;
        description = "Config to use for {file} `mimi/config.toml`.";
      };
      configFile = lib.mkOption {
        type = lib.types.nullOr lib.types.path;
        default = null;
        description = "Path to existing config.toml configuration file. Takes precedence over config option.";
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
          KeepAlive = true;
          RunAtLoad = true;
          EnvironmentVariables = {
            PATH = "/run/current-system/sw/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin";
          };
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
