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
- **Priorities**: Use `A `, `B `, or `C ` at the start of a message (after profile/todo).
    - Input: `todo A fix bug` -> `- TODO [#A] HH:MM fix bug #inbox`
- **Nesting (Also Mode)**:
    - `also [note]` - Nests the note under the previous entry (indented by 2 spaces).
    - `toggle also` - Enables auto-nesting mode for 5 minutes of inactivity.
- **Automatic Tagging**: If a message contains no `#tags`, `#inbox` is automatically appended.
- **URL Title Scraping**: Links are automatically expanded with their page titles.
    - Input: `https://example.com` -> `[Example Domain](https://example.com)`
- **Natural Language Scheduling**:
    - `... scheduled for tomorrow` -> `SCHEDULED: <YYYY-MM-DD Day>`
    - `... deadline next friday` -> `DEADLINE: <YYYY-MM-DD Day>`
- **Media Support**:
    - Photos and Voice Notes are downloaded to the `assets/` folder and linked in the journal.
    - Captions are supported and appended to the link.

### Help
Send `help` to see a general overview, or `help [topic]` for detailed info:
- `help nesting`
- `help priority`
- `help scheduling`
- `help media`

### Bot Feedback
- ✅ **Success**: The bot will reply with the profile used and the exact formatted entry.
- ❌ **Error**: If something goes wrong, the bot will reply with a detailed error message.

## Automation
For a permanent setup, it is recommended to run this script as a systemd service.
