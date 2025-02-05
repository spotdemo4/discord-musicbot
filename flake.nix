{
  description = "discord musicbot";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    gomod2nix = {
      url = "github:nix-community/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    gitignore = {
      url = "github:hercules-ci/gitignore.nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, gomod2nix, gitignore }:
    let
      allSystems = [ 
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs allSystems (system: f {
        inherit system;
        pkgs = import nixpkgs { inherit system; };
      });
    in
    {
      # Dev environment
      devShell = forAllSystems ({ system, pkgs }:
        pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gotools
            gopls
            gomod2nix.legacyPackages.${system}.gomod2nix
            goreleaser
          ];
        }
      );

      # Package
      packages = forAllSystems ({ system, pkgs, ... }:
        let
          buildGoApplication = gomod2nix.legacyPackages.${system}.buildGoApplication;
        in
        rec {
          default = discord-musicbot;

          discord-musicbot = buildGoApplication {
            name = "discord-musicbot";
            src = gitignore.lib.gitignoreSource ./.;
            go = pkgs.go;
            pwd = ./.;
          };
        }
      );

      # Overlay
      overlays.default = final: prev: {
        discord-musicbot = self.packages.${final.stdenv.system}.discord-musicbot;
      };
    };
}
