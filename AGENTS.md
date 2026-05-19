# AI Agent Guidelines

## Scope
- The agent's scope is strictly confined to:
    - Configuration files (`.logseq/*.edn`, `.logseq/*.css`).
    - System documentation (`docs/*.md`, `README.md`, `AGENTS.md`).
    - Scripts and source code (`scripts/`, `flake.nix`).

## Testing
- The `telegram-capture` tool is written in Go.
- Any changes to the capture logic **MUST** be accompanied by corresponding unit tests in `scripts/telegram-capture/bot_test.go`.
- Run `nix flake check` to ensure all tests pass before finalizing changes.

## Strict Prohibitions
- **NEVER** create or modify files within the following subdirectories (personal or work):
    - `journals/`
    - `pages/`
    - `assets/`
- These directories are managed by Syncthing and Logseq; external programmatic modifications risk index corruption or sync conflicts.

## Version Control
- This repository uses Jujutsu (`jj`) with a colocated Git repository.
- Changes to the working copy are snapshotted automatically by `jj`.
- Finalize structural or programmatic refactors with clear intent in the `jj` log.
