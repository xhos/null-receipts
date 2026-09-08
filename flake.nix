{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    git-hooks.url = "github:cachix/git-hooks.nix";
    git-hooks.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = {
    self,
    nixpkgs,
    git-hooks,
  }: let
    systems = ["x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin"];
    forAllSystems = f:
      nixpkgs.lib.genAttrs systems (system: f system nixpkgs.legacyPackages.${system});
  in {
    checks = forAllSystems (system: pkgs: {
      pre-commit = git-hooks.lib.${system}.run {
        src = ./.;
        hooks = {
          alejandra.enable = true;
          # golangci-lint and gotest shell out to `go`, which the hook env lacks
          golangci-lint = {
            enable = true;
            extraPackages = [pkgs.go];
          };

          gotest = {
            enable = true;
            stages = ["pre-push"];
            extraPackages = [pkgs.go];
          };

          nix-build = {
            enable = true;
            entry = pkgs.lib.getExe (pkgs.writeShellApplication {
              name = "nix-build-check";
              runtimeInputs = [pkgs.nix];
              text = "nix build --no-link";
            });
            stages = ["pre-push"];
            pass_filenames = false;
            files = "go\\.(mod|sum)|flake\\.nix";
          };
        };
      };
    });

    packages = forAllSystems (system: pkgs: {
      default = pkgs.buildGoModule {
        pname = "null-receipts";
        version = self.shortRev or self.dirtyShortRev or "dev";
        src = ./.;
        vendorHash = "sha256-GV1DKwZjFrY39GaWtF3JHSg9M/5dSbREB9JC9tOHylw=";
        subPackages = ["cmd/server"];
      };
    });

    devShells = forAllSystems (system: pkgs: {
      default = pkgs.mkShell {
        packages = with pkgs; [
          go
          golangci-lint
          air

          buf
          protoc-gen-go
          protoc-gen-go-grpc
          protoc-gen-connect-go

          ollama

          (writeShellScriptBin "run" ''
            exec ${air}/bin/air -build.cmd "go build -o ./tmp/main ./cmd/server/main.go" -build.bin ./tmp/main
          '')

          (writeShellScriptBin "regen" ''
            rm -rf internal/gen
            ${buf}/bin/buf generate
          '')

          (writeShellScriptBin "bump-protos" ''
            set -e
            git submodule update --remote --checkout proto
            git add proto
            git commit -m "chore: bump protos"
            git push
          '')
        ];

        env.OLLAMA_MODELS = "./models";

        shellHook = self.checks.${system}.pre-commit.shellHook;
      };
    });

    formatter = forAllSystems (system: pkgs: pkgs.alejandra);
  };
}
