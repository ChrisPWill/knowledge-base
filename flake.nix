{
  description = "Logseq Telegram Capture and Zettelkasten Boilerplate";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, utils }:
    utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "logseq-capture";
          version = "0.1.0";
          src = ./apps/logseq-capture;
          vendorHash = "sha256-Vcw4KsE2m27MTEOc+hnkvu3sFmtGyFO+hML3SFksSDU=";
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [ go jq curl ];
        };
      }
    );
}
