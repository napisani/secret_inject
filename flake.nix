
{
  description = "a flake for secret_inject";
  inputs = {
    # golang 1.22
    golang_dep.url = "github:NixOS/nixpkgs/10b813040df67c4039086db0f6eaf65c536886c6";
    # goreleaser 1.24.0
    goreleaser_dep.url = "github:NixOS/nixpkgs/10b813040df67c4039086db0f6eaf65c536886c6";
  };

  outputs = { 
    self, 
    nixpkgs,
    flake-utils, 
    golang_dep,
    goreleaser_dep
  }@inputs :
    flake-utils.lib.eachDefaultSystem (system:
    let
      golang_dep = inputs.golang_dep.legacyPackages.${system};
      goreleaser_dep = inputs.goreleaser_dep.legacyPackages.${system};
    in
    {
      devShells.default = golang_dep.mkShell {
        packages = [
          golang_dep.go_1_22
          golang_dep.gotools
          goreleaser_dep.goreleaser
        ];

        shellHook = ''
          echo "go version: $(go version)"
        '';
      };
    });
}
