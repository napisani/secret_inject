{
  description =
    "secret_inject is a cli for injecting secrets into your shell environment";

  # Nixpkgs / NixOS version to use.
  inputs = {
    nixpkgs.url =
      "github:NixOS/nixpkgs/10b813040df67c4039086db0f6eaf65c536886c6";
    flake-utils.url = "github:numtide/flake-utils";
    goflake.url = "github:sagikazarmark/go-flake";
    goflake.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = { self, nixpkgs, flake-utils, goflake, ... }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        # cpkgs = carlos.packages.${system};

        # pkgs = import nixpkgs {
        #   inherit system;
        #   overlays = [ goflake.overlay ];
        # };
        buildDeps = with pkgs; [ git go_1_22 gnumake ];
        devDeps = with pkgs; buildDeps ++ [ gotools goreleaser ];

        # Generate a user-friendly version number.
        version = builtins.substring 0 8 self.lastModifiedDate;

        # System types to support.
        # supportedSystems = [ "x86_64-linux" "x86_64-darwin" "aarch64-linux" "aarch64-darwin" ];

        # Helper function to generate an attrset '{ x86_64-linux = f "x86_64-linux"; ... }'.
        # forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

        # Nixpkgs instantiated for supported system types.
        # nixpkgsFor = forAllSystems (system: import nixpkgs { inherit system; });
      in {
        packages.default = pkgs.buildGoModule {
          pname = "secret_inject";
          inherit version;
          # In 'nix develop', we don't need a copy of the source tree
          # in the Nix store.
          src = ./.;

          # This hash locks the dependencies of this package. It is
          # necessary because of how Go requires network access to resolve
          # VCS.  See https://www.tweag.io/blog/2021-03-04-gomod2nix/ for
          # details. Normally one can build with a fake sha256 and rely on native Go
          # mechanisms to tell you what the hash should be or determine what
          # it should be "out-of-band" with other tooling (eg. gomod2nix).
          # To begin with it is recommended to set this, but one must
          # remeber to bump this hash when your dependencies change.
          #vendorSha256 = pkgs.lib.fakeSha256;
          nativeBuildInputs = [ buildDeps ];

          vendorHash = "sha256-NzI/Ms98diZFHXeRdEE/XDlGaCOBtBQEogLxAuRZwQQ="; 
        };

        devShell = pkgs.mkShell { buildInputs = devDeps; };
      });

  ## Provide some binary packages for selected system types.
  #packages = forAllSystems (system:
  #  let
  #    pkgs = nixpkgsFor.${system};
  #  in
  #  {
  #    # The default package for 'nix build'. This makes sense if the
  #    # flake provides only one package or there is a clear "main"
  #    # package.
  #    default = pkgs.buildGoModule {
  #      pname = "secret_inject";
  #      inherit version;
  #      # In 'nix develop', we don't need a copy of the source tree
  #      # in the Nix store.
  #      src = ./.;

  #      # This hash locks the dependencies of this package. It is
  #      # necessary because of how Go requires network access to resolve
  #      # VCS.  See https://www.tweag.io/blog/2021-03-04-gomod2nix/ for
  #      # details. Normally one can build with a fake sha256 and rely on native Go
  #      # mechanisms to tell you what the hash should be or determine what
  #      # it should be "out-of-band" with other tooling (eg. gomod2nix).
  #      # To begin with it is recommended to set this, but one must
  #      # remeber to bump this hash when your dependencies change.
  #      #vendorSha256 = pkgs.lib.fakeSha256;

  #      vendorSha256 = pkgs.lib.fakeSha256;
  #    };
  #  });
  #devShell = pkgs.mkShell { buildInputs = devDeps; };
  #});
}
