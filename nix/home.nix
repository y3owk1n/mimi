{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.mimi;
  defaultPath = lib.concatStringsSep ":" (
    [
      "${config.home.homeDirectory}/.nix-profile/bin"
      "/etc/profiles/per-user/${config.home.username}/bin"
      "/run/current-system/sw/bin"
      "/nix/var/nix/profiles/default/bin"
      "/usr/local/bin"
      "/usr/bin"
      "/bin"
    ]
    ++ lib.optionals pkgs.stdenv.isDarwin [ "/opt/homebrew/bin" ]
  );
  # The two console streams launchd captures for this job, named for the daemon.
  #
  # Nothing rotates them, so the daemon empties both once at startup, and these
  # two entries are the whole of what tells it which files those are —
  # internal/daemon/captured_logs.go has the why.
  #
  # Both paths are read back out of the job's own config rather than written
  # here as a second pair of literals. The daemon empties exactly what it is
  # pointed at, so an entry naming a file the job does not write to would
  # destroy one log and leave the real one growing forever. Taking the strings
  # from the option launchd itself is given means an override of either path
  # moves its environment entry with it, and the two cannot come to name
  # different files.
  capturedStreams = {
    MIMI_CAPTURED_STDOUT = config.launchd.agents.mimi.config.StandardOutPath;
    MIMI_CAPTURED_STDERR = config.launchd.agents.mimi.config.StandardErrorPath;
  };
  effectiveEnv = {
    PATH = defaultPath;
  }
  // cfg.extraEnvironment
  // capturedStreams;
in
{
  options = {
    services.mimi = {
      enable = lib.mkEnableOption "Mimi window and space utility daemon";

      package = lib.mkPackageOption pkgs "mimi" { };

      config = lib.mkOption {
        type = lib.types.lines;
        default = builtins.readFile ../configs/default-config.toml;
        description = ''
          Configuration for {file} `mimi/config.toml`.
        '';
      };

      configFile = lib.mkOption {
        type = lib.types.nullOr lib.types.path;
        default = null;
        description = "Path to existing config.toml configuration file. Takes precedence over config option.";
      };

      launchd = {
        enable = lib.mkOption {
          type = lib.types.bool;
          default = true;
          description = ''
            Configure the launchd agent to manage the Mimi process.

            The first time this is enabled, macOS will prompt you to allow this background
            item in System Settings.

            You can verify the service is running correctly from your terminal.
            Run: `launchctl list | grep mimi`

            - A running process will show a Process ID (PID) and a status of 0, for example:
              `12345	0	org.nix-community.home.mimi`

            - If the service has crashed or failed to start, the PID will be a dash and the
              status will be a non-zero number, for example:
              `-	1	org.nix-community.home.mimi`

            If the app fails to launch at all, check `cat /tmp/mimi.err.log` for launch errors.

            For more detailed service status, run `launchctl print gui/$(id -u)/org.nix-community.home.mimi`.
          '';
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
          PATH = "/run/current-system/sw/bin:/nix/var/nix/profiles/default/bin:/usr/local/bin:/usr/bin:/bin";
        };
        description = ''
          Additional environment variables to set in the launchd (macOS) or systemd (Linux) service.
          These are merged with defaults such as a {env}`PATH`
          that includes common Nix binary directories and the user's Nix profile.
          Setting {env}`PATH` here will override the default entirely.

          Setting {env}`MIMI_CAPTURED_STDOUT` or {env}`MIMI_CAPTURED_STDERR`
          here has no effect. The module always fills those two from the paths
          launchd writes the captured streams to, which is what tells the daemon
          which files to empty at each start. Move a stream with
          {option}`launchd.agents.mimi.config.StandardOutPath` or
          {option}`launchd.agents.mimi.config.StandardErrorPath` and its
          environment entry follows.

          To extend the default PATH with additional directories:
          ```nix
          services.mimi.extraEnvironment = {
            PATH = "/Users/me/.cargo/bin:/Users/me/.nix-profile/bin:/etc/profiles/per-user/me/bin:/run/current-system/sw/bin:/nix/var/nix/profiles/default/bin:/usr/local/bin:/usr/bin:/bin";
          };
          ```
        '';
      };
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [ cfg.package ];

    # Generate config file - either from text or source file
    xdg.configFile."mimi/config.toml" =
      if cfg.configFile != null then { source = cfg.configFile; } else { text = cfg.config; };

    # Launch agent for macOS
    launchd.agents.mimi = {
      enable = cfg.launchd.enable;
      config = {
        ProgramArguments = [
          "${cfg.package}/Applications/Mimi.app/Contents/MacOS/mimi"
          "start"
          "--config"
          "${config.xdg.configHome}/mimi/config.toml"
        ];
        EnvironmentVariables = effectiveEnv;
        RunAtLoad = true;
        KeepAlive = cfg.launchd.keepAlive;
        StandardOutPath = "/tmp/mimi.log";
        StandardErrorPath = "/tmp/mimi.err.log";
        ProcessType = "Interactive";
        LimitLoadToSessionType = "Aqua";
        Nice = -10;
        ThrottleInterval = 10;
      };
    };
  };
}
