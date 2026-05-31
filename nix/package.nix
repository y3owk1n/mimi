{
  fetchurl,
  gitUpdater,
  installShellFiles,
  stdenv,
  versionCheckHook,
  lib,
  buildGoModule,
  version ? "main",
  useZip ? false,
  commitHash ? null,
  writableTmpDirAsHomeHook,
  nix-update-script,
  unzip,
}:
if useZip then
  let
    appName = "Mimi.app";

    # Determine architecture-specific details
    archInfo =
      {
        "aarch64-darwin" = {
          url = "https://github.com/y3owk1n/mimi/releases/download/v${version}/mimi-darwin-arm64.zip";
          # run `nix hash convert --hash-algo sha256 (nix-prefetch-url https://github.com/y3owk1n/mimi/releases/download/v0.1.0/mimi-darwin-arm64.zip)`
          sha256 = "";
        };
        "x86_64-darwin" = {
          url = "https://github.com/y3owk1n/mimi/releases/download/v${version}/mimi-darwin-amd64.zip";
          # run `nix hash convert --hash-algo sha256 (nix-prefetch-url https://github.com/y3owk1n/mimi/releases/download/v0.1.0/mimi-darwin-amd64.zip)`
          sha256 = "";
        };
      }
      .${stdenv.hostPlatform.system} or (throw "Unsupported system: ${stdenv.hostPlatform.system}");

  in
  stdenv.mkDerivation {
    pname = "mimi";

    inherit version;

    src = fetchurl {
      url = archInfo.url;
      sha256 = archInfo.sha256;
    };

    unpackPhase = ''
      unzip $src
    '';

    nativeBuildInputs = [
      installShellFiles
      unzip
    ];

    installPhase = ''
      runHook preInstall
      ${
        if stdenv.hostPlatform.isDarwin then
          ''
            mkdir -p $out/Applications
            mv ${appName} $out/Applications
            cp -R bin $out
            mkdir -p $out/share/man/man1
            mv share/man/man1/*.1 $out/share/man/man1/
          ''
        else
          ''
            mkdir -p $out/bin
            mv bin/mimi $out/bin/mimi
            mkdir -p $out/share/man/man1
            mv share/man/man1/*.1 $out/share/man/man1/
          ''
      }
      runHook postInstall
    '';

    postInstall = ''
      if ${
        lib.boolToString (
          stdenv.buildPlatform.canExecute stdenv.hostPlatform && stdenv.hostPlatform.isDarwin
        )
      }; then
        installShellCompletion --cmd mimi \
              --bash <($out/Applications/Mimi.app/Contents/MacOS/mimi completion bash) \
              --fish <($out/Applications/Mimi.app/Contents/MacOS/mimi completion fish) \
              --zsh <($out/Applications/Mimi.app/Contents/MacOS/mimi completion zsh)
      fi
    '';

    doInstallCheck = true;
    nativeInstallCheckInputs = [
      versionCheckHook
    ];

    passthru.updateScript = gitUpdater {
      url = "https://github.com/y3owk1n/mimi.git";
      rev-prefix = "v";
    };

    meta = with lib; {
      description = "Listen to macOS system events and react";
      homepage = "https://github.com/y3owk1n/mimi";
      license = licenses.mit;
      platforms = platforms.darwin;
      mainProgram = "mimi";
    };
  }
else
  let
    shortHash = if commitHash != null then lib.substring 0 7 commitHash else null;

    pversion = "${version}${if shortHash != null then "-${shortHash}" else ""}";
  in
  # Build from source
  buildGoModule (finalAttrs: {
    pname = "mimi";
    version = pversion;

    src = lib.cleanSource ../.;

    # run the following command to get the sha256 hash
    # `nix-shell -p go --run 'go mod vendor'`
    # `nix hash path vendor`
    # `rm -rf vendor`
    vendorHash = "sha256-hS1EIacvDfTp6G50aegOxlBnvjrHYDz8TqsUbZ3Xm6o=";

    ldflags = [
      "-s"
      "-w"
      "-X github.com/y3owk1n/mimi/cmd/mimi/cmd.Version=${finalAttrs.version}"
    ]
    ++ lib.optionals (commitHash != null) [
      "-X github.com/y3owk1n/mimi/cmd/mimi/cmd.GitCommit=${commitHash}"
    ];

    nativeBuildInputs = [
      installShellFiles
      writableTmpDirAsHomeHook
    ];

    subPackages = [ "cmd/mimi" ];

    # Allow Go to use any available toolchain
    preBuild = ''
      export GOTOOLCHAIN=auto
    '';

    postInstall = ''
      # generate man pages
      mkdir -p $out/share/man/man1
      go run ./cmd/genman $out/share/man/man1

      # install shell completions
      if ${lib.boolToString (stdenv.buildPlatform.canExecute stdenv.hostPlatform)}; then
      	installShellCompletion --cmd mimi \
      	--bash <($out/bin/mimi completion bash) \
      	--fish <($out/bin/mimi completion fish) \
      	--zsh <($out/bin/mimi completion zsh)
      fi
    ''
    + lib.optionalString stdenv.hostPlatform.isDarwin ''
      # Create a simple .app bundle on the fly for macOS source builds.
      mkdir -p $out/Applications/Mimi.app/Contents/{MacOS,Resources}

      cp $out/bin/mimi $out/Applications/Mimi.app/Contents/MacOS/mimi

      # cp ${finalAttrs.src}/resources/icon.icns $out/Applications/Mimi.app/Contents/Resources/icon.icns

      SRC_PLIST=${finalAttrs.src}/resources/Info.plist.template

      sed "s|VERSION|${finalAttrs.version}|g" $SRC_PLIST > $out/Applications/Mimi.app/Contents/Info.plist

      echo "✅ Mimi.app bundle created at $out/Applications/Mimi.app"
    '';

    passthru = {
      updateScript = nix-update-script { };
    };

    meta = with lib; {
      description = "Listen to macOS system events and react";
      homepage = "https://github.com/y3owk1n/mimi";
      license = licenses.mit;
      platforms = platforms.darwin;
      mainProgram = "mimi";
    };
  })
