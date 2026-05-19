# Mobile Capture via Telegram

This project includes a lightweight Telegram bot capture system that allows you to send quick notes to your Logseq journals from your phone.

## Setup Instructions

### 1. Create a Telegram Bot
1. Message [@BotFather](https://t.me/botfather) on Telegram.
2. Send `/newbot` and follow the prompts to get your **Bot Token**.
3. (Optional but recommended) Set a description and profile picture for your bot.

### 2. Configure Secrets
There are two ways to provide the bot token:

#### A. Via Home Manager (Recommended)
If you are using Nix and Home Manager, you can inject the `TELEGRAM_BOT_TOKEN` environment variable into the service or shell environment where the script runs.

#### B. Local .env File
Create a `.env` file in the root of this repository (this file is ignored by git):
```bash
TELEGRAM_BOT_TOKEN="your_token_here"
```

### 3. Running the Capture Script

#### Using Nix (Recommended)
You can run the script directly using Nix:
```bash
nix run .
```

#### Manual Run
Ensure you have `curl` and `jq` installed, then:
```bash
./scripts/telegram-capture.sh
```

## Usage

Send a message to your bot to capture it. By default, messages go to your **personal** journal.

- `/w [note]` or `/work [note]` - Captures to `work/journals/YYYY_MM_DD.md`.
- `/p [note]` or `/personal [note]` - Captures to `personal/journals/YYYY_MM_DD.md`.
- `[note]` (no prefix) - Defaults to **personal**.

### Example
- `/w Discuss project timeline with team` -> Appends `- HH:MM Discuss project timeline with team` to today's work journal.

## Automation
For a permanent setup, it is recommended to run this script as a systemd service (managed via Home Manager or NixOS) so it is always listening for captures in the background.
