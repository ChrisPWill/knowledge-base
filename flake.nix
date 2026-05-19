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
        deps = with pkgs; [ bash curl jq coreutils ];
      in
      {
        packages.default = pkgs.writeShellApplication {
          name = "telegram-capture";
          runtimeInputs = deps;
          text = builtins.readFile ./scripts/telegram-capture.sh;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = deps;
        };
      }
    );
}
