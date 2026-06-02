# Contributing to Knowledge Base

Thank you for your interest in contributing! This project uses a local-first workflow combining Logseq, Syncthing, and a custom Go-based Telegram capture bot.

## Development Environment

This project uses [Nix](https://nixos.org/) for development environment management. 

1.  **Enter the Dev Shell:**
    ```bash
    nix develop
    ```
    This provides `go`, `jq`, and `curl`.

2.  **Run Tests:**
    Before submitting changes to the Telegram bot, ensure all tests pass:
    ```bash
    cd apps/logseq-capture
    go test ./...
    ```

3.  **Check Flake Integrity:**
    ```bash
    nix flake check
    ```

## Structure

- `apps/logseq-capture`: The Go source for the Telegram capture bot.
- `personal/logseq` & `work/logseq`: Logseq configuration boilerplates.
- `docs/`: System documentation.
- `scripts/`: Utility scripts.

## Standards

- **Privacy First:** Never commit files inside `journals/`, `pages/`, or `assets/`.
- **Testing:** New features in the capture bot MUST include unit tests in `bot_test.go` or relevant `*_test.go` files.
- **Commit Messages:** Follow a clear and descriptive style for structural changes.
