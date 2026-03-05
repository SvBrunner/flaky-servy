{
  description = "GO Dev environment for flaky-servy";

  inputs = {
    flake-parts.url = "github:hercules-ci/flake-parts";
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { flake-parts, ... }@inputs:
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];

      perSystem =
        { pkgs, system, ... }:
        {
          devShells.default = pkgs.mkShell {
            packages = with pkgs; [
              go
              gopls
            ];

            env = { 
              GOPATH = "/home/sven/go";
            };

            shellHook = ''
              go version
            '';
          };
        };
    };
}
