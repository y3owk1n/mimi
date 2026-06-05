{
  description = "Listen to macOS system events and react";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs, ... }:
    let
      eachSystem = nixpkgs.lib.genAttrs [
        "aarch64-darwin"
        "x86_64-darwin"
      ];

      # Update this to your latest release version
      latestVersion = "0.4.0";

      # Function to build package with specific version
      makeMimiPackage =
        pkgs: version: useZip: commitHash:
        pkgs.callPackage ./nix/package.nix {
          inherit version useZip commitHash;
        };
    in
    {
      overlays.default = final: prev: {
        mimi = makeMimiPackage final latestVersion true null;
        mimi-source = makeMimiPackage final "main" false (self.rev or self.dirtyRev or "unknown");
      };

      # Packages output using the overlay
      packages = eachSystem (
        system:
        let
          pkgs = import nixpkgs {
            inherit system;
            overlays = [ self.overlays.default ];
          };
        in
        {
          # Default: latest version from zip for all supported platforms
          default = makeMimiPackage pkgs latestVersion true null;

          # Build from source (for users who want to build from source)
          source = makeMimiPackage pkgs "main" false (self.rev or self.dirtyRev or "unknown");
        }
      );

      darwinModules.default = import ./nix/darwin.nix;
      homeManagerModules.default = import ./nix/home.nix;
    };
}
