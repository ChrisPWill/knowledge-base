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
        packages = rec {
          default = logseq-capture;

          logseq-capture = pkgs.buildGoModule {
            pname = "logseq-capture";
            version = "0.1.0";
            src = ./apps/logseq-capture;
            vendorHash = "sha256-Vcw4KsE2m27MTEOc+hnkvu3sFmtGyFO+hML3SFksSDU=";
          };

          knowledge-base-summary = pkgs.buildGoModule {
            pname = "knowledge-base-summary";
            version = "0.1.0";
            src = ./apps/knowledge-base-summary;
            vendorHash = null;
            nativeBuildInputs = [ pkgs.makeWrapper ];
            postInstall = ''
              wrapProgram "$out/bin/knowledge-base-summary" \
                --prefix PATH : ${pkgs.lib.makeBinPath [ pkgs.ripgrep ]}
            '';
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [ go jq curl ripgrep ];
        };
      })
    // {
      homeManagerModules.knowledge-base = import ./nix/home-manager/knowledge-base.nix { inherit self; };
    };
}
