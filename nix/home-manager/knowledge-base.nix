{ self }:
{ config, lib, pkgs, ... }:

let
  cfg = config.programs."knowledge-base".logseqShellSummary;
  package = cfg.package;
  cachePath = cfg.cachePath;
  displayStatePath = cfg.displayStatePath;
  refreshCommand = lib.escapeShellArgs (
    [
      "${package}/bin/knowledge-base-summary"
      "--personal-path" cfg.personalPath
      "--work-path" cfg.workPath
      "--cache-path" cachePath
      "--max-digest-items" (toString cfg.maxDigestItems)
      "--excerpt-length" (toString cfg.excerptLength)
    ]
    ++ lib.concatMap (tag: [ "--count-only-tag" tag ]) cfg.countOnlyTags
    ++ lib.concatMap (tag: [ "--digest-tag" tag ]) cfg.digestTags
  );
  dateBin = "${pkgs.coreutils}/bin/date";
  catBin = "${pkgs.coreutils}/bin/cat";
  mkdirBin = "${pkgs.coreutils}/bin/mkdir";
  dirnameBin = "${pkgs.coreutils}/bin/dirname";
  posixShellHook = ''
    if [ -r "${cachePath}" ]; then
      __kb_now="$(${dateBin} +%s)"
      __kb_last=""

      if [ -r "${displayStatePath}" ]; then
        __kb_last="$(${catBin} "${displayStatePath}" 2>/dev/null)"
      fi

      case "$__kb_last" in
        ''|*[!0-9]*)
          __kb_last=0
          ;;
      esac

      if [ "$((__kb_now - __kb_last))" -ge "${toString cfg.intervalSeconds}" ]; then
        ${mkdirBin} -p "$(${dirnameBin} "${displayStatePath}")"
        printf '%s\n' "$__kb_now" > "${displayStatePath}"
        ${catBin} "${cachePath}"
      fi
    fi
  '';
  fishShellHook = ''
    if test -r "${cachePath}"
      set -l __kb_now (${
        dateBin
      } +%s)
      set -l __kb_last 0

      if test -r "${displayStatePath}"
        set __kb_last (string trim -- (${
          catBin
        } "${displayStatePath}" 2>/dev/null))
      end

      if not string match -rq '^[0-9]+$' -- "$__kb_last"
        set __kb_last 0
      end

      if test (math "$__kb_now - $__kb_last") -ge ${toString cfg.intervalSeconds}
        ${mkdirBin} -p (${dirnameBin} "${displayStatePath}")
        printf '%s\n' "$__kb_now" > "${displayStatePath}"
        ${catBin} "${cachePath}"
      end
    end
  '';
in
{
  options.programs."knowledge-base".logseqShellSummary = {
    enable = lib.mkEnableOption "shell summaries for Logseq tags";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.system}.knowledge-base-summary;
      defaultText = lib.literalExpression "self.packages.${pkgs.system}.knowledge-base-summary";
      description = "The package that builds the summary cache.";
    };

    personalPath = lib.mkOption {
      type = lib.types.str;
      example = "/home/alice/knowledge-base/personal";
      description = "Path to the personal knowledge base root.";
    };

    workPath = lib.mkOption {
      type = lib.types.str;
      example = "/home/alice/knowledge-base/work";
      description = "Path to the work knowledge base root.";
    };

    countOnlyTags = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      description = "Tags that should only show counts.";
    };

    digestTags = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      description = "Tags that should show counts and excerpts.";
    };

    intervalSeconds = lib.mkOption {
      type = lib.types.ints.positive;
      default = 3600;
      description = "How often to refresh the summary cache, in seconds.";
    };

    maxDigestItems = lib.mkOption {
      type = lib.types.ints.positive;
      default = 5;
      description = "Maximum excerpts to show for each digest tag.";
    };

    excerptLength = lib.mkOption {
      type = lib.types.ints.positive;
      default = 160;
      description = "Maximum excerpt length for digest output.";
    };

    cachePath = lib.mkOption {
      type = lib.types.str;
      default = "${config.home.homeDirectory}/.cache/knowledge-base/logseq-shell-summary/summary.txt";
      description = "Path to the rendered summary cache.";
    };

    displayStatePath = lib.mkOption {
      type = lib.types.str;
      default = "${config.home.homeDirectory}/.cache/knowledge-base/logseq-shell-summary/last-displayed";
      description = "Path to the shell hook display-rate state file.";
    };

    shells = {
      bash.enable = lib.mkOption {
        type = lib.types.bool;
        default = true;
        description = "Enable the shell summary hook for Bash.";
      };

      fish.enable = lib.mkOption {
        type = lib.types.bool;
        default = true;
        description = "Enable the shell summary hook for Fish.";
      };

      zsh.enable = lib.mkOption {
        type = lib.types.bool;
        default = true;
        description = "Enable the shell summary hook for Zsh.";
      };
    };
  };

  config = lib.mkIf cfg.enable (lib.mkMerge [
    {
      assertions = [
        {
          assertion = cfg.countOnlyTags != [ ] || cfg.digestTags != [ ];
          message = "programs.knowledge-base.logseqShellSummary requires at least one tag.";
        }
      ];
    }

    (lib.mkIf (pkgs.stdenv.isLinux) {
      systemd.user.services.knowledge-base-logseq-shell-summary = {
        Unit.Description = "Refresh the cached knowledge-base shell summary";
        Service = {
          Type = "oneshot";
          ExecStart = "${pkgs.runtimeShell} -lc ${lib.escapeShellArg refreshCommand}";
        };
      };

      systemd.user.timers.knowledge-base-logseq-shell-summary = {
        Unit.Description = "Refresh the cached knowledge-base shell summary";
        Timer = {
          OnBootSec = "2m";
          OnUnitActiveSec = "${toString cfg.intervalSeconds}s";
          Unit = "knowledge-base-logseq-shell-summary.service";
        };
        Install.WantedBy = [ "timers.target" ];
      };
    })

    (lib.mkIf (pkgs.stdenv.isDarwin) {
      launchd.agents.knowledge-base-logseq-shell-summary = {
        enable = true;
        config = {
          ProgramArguments = [ pkgs.runtimeShell "-lc" refreshCommand ];
          RunAtLoad = true;
          StartInterval = cfg.intervalSeconds;
          StandardErrorPath = "${config.home.homeDirectory}/Library/Logs/knowledge-base-logseq-shell-summary.err.log";
          StandardOutPath = "${config.home.homeDirectory}/Library/Logs/knowledge-base-logseq-shell-summary.out.log";
        };
      };
    })

    (lib.mkIf (cfg.shells.bash.enable && (config.programs.bash.enable or false)) {
      programs.bash.initExtra = posixShellHook;
    })

    (lib.mkIf (cfg.shells.fish.enable && (config.programs.fish.enable or false)) {
      programs.fish.interactiveShellInit = fishShellHook;
    })

    (lib.mkIf (cfg.shells.zsh.enable && (config.programs.zsh.enable or false)) {
      programs.zsh.initContent = posixShellHook;
    })
  ]);
}
