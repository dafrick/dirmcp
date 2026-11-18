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
    in
    {
      formatter = forAllSystems (pkgs: pkgs.nixfmt);

      checks = forAllSystems (pkgs: {
        hooks = git-hooks.lib.${pkgs.stdenv.hostPlatform.system}.run {
          src = ./.;
          hooks = {
            nixfmt.enable = true;
            gitleaks.enable = true;
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
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          inherit (self.checks.${pkgs.stdenv.hostPlatform.system}.hooks) shellHook;
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
