{
  description = "Logseq Telegram Capture and Zettelkasten Boilerplate";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    utils.url = "github:numtide/flake-utils";
  };

  outputs = { nixpkgs, utils }:
    utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "telegram-capture";
          version = "0.1.0";
          src = ./scripts/telegram-capture;
          vendorHash = null;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [ go jq curl ];
        };
      }
    );
}
