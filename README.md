# Logseq Zettelkasten Boilerplate (Local-First)

This repository provides a structured boilerplate for a local-first Zettelkasten system using Logseq. Note content is synced between other devices and mobile using Syncthing. It supports note capture via a smart Telegram bot.

## Structure
- `personal/`: Logseq graph for personal notes.
- `work/`: Logseq graph for professional notes.
- `docs/`: System documentation and guides.
- `scripts/`: Utility scripts for system health.

## Key Features
- **Local-First Sync**: Uses Syncthing for private, peer-to-peer note synchronization across devices.
- **Mobile Capture**: Integrated Telegram bot for capturing notes, TODOs, and media directly to your journals.
- **Smart Formatting**: Automatic TODO creation, priority handling, and natural language scheduling (e.g., "scheduled for tomorrow").
- **Zettelkasten Workflow**: Pre-configured structure for Literature, Permanent, and Structure notes (MOCs).
- **Privacy-First**: Note content is ignored by Git/Jujutsu; only system configurations and documentation are tracked.

## Getting Started

1. **Initialize the Repository**:
   ```bash
   jj git init --colocate
   ```

2. **Configure Logseq**:
   - Open Logseq.
   - Click "Add new graph".
   - Select the `personal/` or `work/` directory.
   - Logseq will use the `.logseq/` settings already in those folders.

3. **Setup Syncthing**:
   - Add the `personal/` and `work/` folders to Syncthing.
   - Share them across your devices (Desktop, Laptop, Mobile).
   - Ensure `journals/`, `pages/`, and `assets/` are synced, but `.git/` and `.jj/` are **not** shared via Syncthing (they are managed by `jj`).

## Version Control with Jujutsu (jj)

This repo uses a colocated Git/Jujutsu setup. Use `jj` for daily operations:

- `jj st`: View status.
- `jj log`: View revision history.
- `jj desc -m "Update config"`: Add a message to the current change.
- `jj git push`: Push changes to a remote Git server.

## Health Check
Run the health check script to ensure no sensitive note content is being accidentally tracked:
```bash
./scripts/health-check.sh
```

## Home Manager Shell Summary

This flake also exposes `homeManagerModules.knowledge-base` for an optional shell-startup summary of tagged Logseq content.

Example Home Manager configuration:
```nix
{
  imports = [ inputs.knowledge-base.homeManagerModules.knowledge-base ];

  programs."knowledge-base".logseqShellSummary = {
    enable = true;
    personalPath = "/home/alice/knowledge-base/personal";
    workPath = "/home/alice/knowledge-base/work";
    countOnlyTags = [ "private" ];
    digestTags = [ "project/foo" "ops+prod" ];
    intervalSeconds = 3600;
  };
}
```

The expensive search runs in a background scheduler and the shell hook only prints the cached summary, so interactive shell startup stays fast.

## Privacy & Security

This repository is a **structural boilerplate**. 
- The `.gitignore` is configured to exclude all note content (`journals/`, `pages/`, `assets/`).
- Only configuration files (`.logseq/*.edn`, `.logseq/*.css`) and documentation are tracked.
- **Never** disable the `health-check.sh` script if you plan to contribute, as it protects against accidental content leaks.
