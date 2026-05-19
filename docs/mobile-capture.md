# Mobile Capture via Telegram

This project includes a lightweight Telegram bot capture system that allows you to send quick notes to your Logseq journals from your phone.

## Setup Instructions

### 1. Create a Telegram Bot
1. Message [@BotFather](https://t.me/botfather) on Telegram.
2. Send `/newbot` and follow the prompts to get your **Bot Token**.
3. (Optional but recommended) Set a description and profile picture for your bot.

### 2. Configure Secrets
The bot requires the `TELEGRAM_BOT_TOKEN` environment variable.

#### A. Nix/Direnv (Recommended)
If you use `direnv`, create a `.envrc` file:
```bash
export TELEGRAM_BOT_TOKEN="your_token_here"
```

#### B. Manual Export
```bash
export TELEGRAM_BOT_TOKEN="your_token_here"
```

### 3. Running the Capture Script

#### Using Nix (Recommended)
You can run the script directly from the root of the repo:
```bash
nix run .
```

#### Manual Run
Ensure you have Go installed, then:
```bash
cd scripts/telegram-capture && go run .
```

## Usage

Send a message to your bot to capture it. The bot provides immediate feedback for every message.

### Profile Selection
- `/w [note]` or `/work [note]` - Captures to `work/journals/YYYY_MM_DD.md`.
- `/p [note]` or `/personal [note]` - Captures to `personal/journals/YYYY_MM_DD.md`.
- `[note]` (no prefix) - Defaults to **personal**.

### Smart Features
- **TODOs**: Start a message with `todo ` (case-insensitive) to create a Logseq TODO.
    - Input: `todo Buy milk`
    - Result: `- TODO HH:MM Buy milk #inbox`
- **Automatic Tagging**: If a message contains no `#tags`, `#inbox` is automatically appended.
    - Input: `Meeting with Sarah`
    - Result: `- HH:MM Meeting with Sarah #inbox`
    - Input: `Important #idea`
    - Result: `- HH:MM Important #idea` (no `#inbox` added)

### Bot Feedback
- ✅ **Success**: The bot will reply with the profile used and the exact formatted entry.
- ❌ **Error**: If something goes wrong (e.g., directory missing), the bot will reply with a detailed error message.

## Automation
For a permanent setup, it is recommended to run this script as a systemd service (managed via Home Manager or NixOS). This ensures it is always listening in the background and starts automatically on boot.
