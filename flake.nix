{
  description =
    "secret_inject is a cli for injecting secrets into your shell environment";

  # Nixpkgs / NixOS version to use.
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        buildDeps = with pkgs; [ git go gnumake ];
        devDeps = with pkgs; buildDeps ++ [ gotools goreleaser ];

        # Generate a user-friendly version number.
        version = builtins.substring 0 8 self.lastModifiedDate;

      in {
        packages.default = pkgs.buildGoModule {
          pname = "secret_inject";
          inherit version;
          src = ./.;
          subPackages = [ "cmd/secret_inject" ];

          # This hash locks the dependencies of this package. It is
          # necessary because of how Go requires network access to resolve
          # VCS.  See https://www.tweag.io/blog/2021-03-04-gomod2nix/ for
          # details. Normally one can build with a fake sha256 and rely on native Go
          # mechanisms to tell you what the hash should be or determine what
          # it should be "out-of-band" with other tooling (eg. gomod2nix).
          # To begin with it is recommended to set this, but one must
          # remeber to bump this hash when your dependencies change.
          #vendorSha256 = pkgs.lib.fakeSha256;

          # Set to the actual hash after first build, or run `nix build 2>&1 | grep "got:"`
          # to extract the correct hash
          vendorHash = "sha256-zA/zROyKdMiwa/jcWMrFSF/uVuRi1zWsPhYLYSaPtaw=";
        };

        devShells.default = pkgs.mkShell { buildInputs = devDeps; };
      });
}
