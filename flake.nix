{
  description = "dirmcp: MCP server for remote, shell-like access to one directory";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    git-hooks = {
      url = "github:cachix/git-hooks.nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      git-hooks,
    }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      linux = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forLinux = f: nixpkgs.lib.genAttrs linux (system: f nixpkgs.legacyPackages.${system});

      # The programs `sh` may start, by name. Their store paths and closure
      # go into the tools manifest; the binary never searches PATH.
      toolsManifest =
        pkgs:
        let
          bin = pkg: names: map (n: nixpkgs.lib.nameValuePair n "${pkg}/bin/${n}") names;
          programs = builtins.listToAttrs (
            bin pkgs.coreutils [
              "ls"
              "cat"
              "head"
              "tail"
              "wc"
              "sort"
              "uniq"
              "cut"
              "tr"
              "paste"
              "comm"
              "join"
              "nl"
              "fold"
              "fmt"
              "tac"
              "stat"
              "du"
              "df"
              "basename"
              "dirname"
              "realpath"
              "readlink"
              "mkdir"
              "mv"
              "cp"
              "touch"
              "date"
              "md5sum"
              "sha256sum"
            ]
            ++ bin pkgs.gnugrep [ "grep" ]
            ++ bin pkgs.ripgrep [ "rg" ]
            ++ bin pkgs.gnused [ "sed" ]
            ++ bin pkgs.gawk [ "gawk" ]
            ++ bin pkgs.findutils [ "find" ]
            ++ bin pkgs.diffutils [
              "diff"
              "cmp"
            ]
            ++ bin pkgs.tree [ "tree" ]
            ++ bin pkgs.file [ "file" ]
          );
          closure = pkgs.closureInfo {
            rootPaths = [
              pkgs.coreutils
              pkgs.gnugrep
              pkgs.ripgrep
              pkgs.gnused
              pkgs.gawk
              pkgs.findutils
              pkgs.diffutils
              pkgs.tree
              pkgs.file
            ];
          };
        in
        pkgs.writeText "dirmcp-tools.json" (
          builtins.toJSON {
            inherit programs;
            closure = "${closure}/store-paths";
          }
        );

      package =
        pkgs:
        pkgs.buildGoModule {
          pname = "dirmcp";
          version = nixpkgs.lib.trim (builtins.readFile ./VERSION);
          src = nixpkgs.lib.cleanSource ./.;
          vendorHash = "sha256-nLJNxHh6TLBe1OfjfRPesLe8ynNdhxG4Mgnm0fX2eIw=";
          env.CGO_ENABLED = 0; # Landlock needs all-threads syscalls, not cgo
          subPackages = [ "cmd/dirmcp" ];
          ldflags = [
            "-s"
            "-w"
            "-X main.version=${nixpkgs.lib.trim (builtins.readFile ./VERSION)}"
            "-X main.defaultTools=${toolsManifest pkgs}"
          ];
          # All packages' tests, with the real tools under Landlock.
          # DIRMCP_REQUIRE_LANDLOCK turns "skip without Landlock" into a failure.
          checkPhase = ''
            runHook preCheck
            export DIRMCP_TOOLS=${toolsManifest pkgs} DIRMCP_REQUIRE_LANDLOCK=1
            go test -count=1 ./...
            runHook postCheck
          '';
          meta = {
            description = "MCP server for remote, shell-like access to one directory";
            homepage = "https://github.com/dafrick/dirmcp";
            license = nixpkgs.lib.licenses.mit;
            mainProgram = "dirmcp";
            platforms = nixpkgs.lib.platforms.linux;
          };
        };
    in
    {
      formatter = forAllSystems (pkgs: pkgs.nixfmt);

      packages = forLinux (pkgs: {
        default = package pkgs;
        tools-manifest = toolsManifest pkgs;
      });

      checks =
        nixpkgs.lib.recursiveUpdate
          (forLinux (pkgs: {
            # go test runs in the package build (preCheck sets DIRMCP_TOOLS).
            package = package pkgs;
          }))
          (
            forAllSystems (pkgs: {
              hooks = git-hooks.lib.${pkgs.stdenv.hostPlatform.system}.run {
                src = ./.;
                hooks = {
                  nixfmt.enable = true;
                  gitleaks.enable = true;
                  gofmt.enable = true;
                  govet = {
                    enable = true;
                    # vet needs the module dependencies, which the sandbox lacks.
                    stages = [ "pre-push" ];
                  };
                  # Full-history secret scan. pre-push only: the sandboxed
                  # `nix flake check` has no real history to scan.
                  gitleaks-history = {
                    enable = true;
                    name = "gitleaks (history)";
                    entry = "${pkgs.gitleaks}/bin/gitleaks git --redact --verbose";
                    pass_filenames = false;
                    always_run = true;
                    stages = [ "pre-push" ];
                  };
                  release-check = {
                    enable = true;
                    name = "release check";
                    entry = "scripts/check-release.sh";
                    pass_filenames = false;
                    always_run = true;
                    stages = [ "pre-push" ];
                  };
                };
              };
            })
          );

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          inherit (self.checks.${pkgs.stdenv.hostPlatform.system}.hooks) shellHook;
          # Tests and `go generate` use the packaged tools (Linux only).
          env = nixpkgs.lib.optionalAttrs pkgs.stdenv.isLinux { DIRMCP_TOOLS = toolsManifest pkgs; };
          packages = self.checks.${pkgs.stdenv.hostPlatform.system}.hooks.enabledPackages ++ [
            pkgs.go
            pkgs.gopls
            pkgs.gh
            pkgs.openspec
          ];
        };
      });
    };
}
