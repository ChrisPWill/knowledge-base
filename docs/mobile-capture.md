# Mobile Capture

This project includes a transport-agnostic Logseq capture service. The default runtime starts both Telegram polling and the local HTTP daemon in one process, and the CLI is available for desktop capture and automation.

## Setup Instructions

### 1. Create a Telegram Bot
1. Message [@BotFather](https://t.me/botfather) on Telegram.
2. Send `/newbot` and follow the prompts to get your **Bot Token**.
3. (Optional but recommended) Set a description and profile picture for your bot.

### 2. Configure Secrets
The default Telegram runtime requires the `LOGSEQ_CAPTURE_TELEGRAM_API_KEY` environment variable.
Unprefixed captures default to `personal` unless you set `LOGSEQ_CAPTURE_DEFAULT_JOURNAL=work`.

#### A. Nix/Direnv (Recommended)
If you use `direnv`, create a `.envrc` file with:
```bash
use flake
```

Then export `LOGSEQ_CAPTURE_TELEGRAM_API_KEY` in your shell or a separate local secrets file.

#### B. Manual Export
```bash
export LOGSEQ_CAPTURE_TELEGRAM_API_KEY="your_token_here"
export LOGSEQ_CAPTURE_DEFAULT_JOURNAL="work"
```

### 3. Running The Default Combined Runtime

#### Using Nix
```bash
nix run .
```

#### Manual Run
```bash
cd apps/logseq-capture && go run .
```

These commands now start:
- Telegram polling
- The localhost HTTP API on `127.0.0.1:43123`

If you want Telegram-only behavior, use:
```bash
cd apps/logseq-capture && go run . telegram
```

## Local Daemon And CLI

The local daemon binds to `127.0.0.1:43123` by default. Override it with `LOGSEQ_CAPTURE_ADDR` if needed.

Start the daemon:
```bash
cd apps/logseq-capture && go run . serve
```

Check daemon health:
```bash
cd apps/logseq-capture && go run . status
```

Send text capture requests:
```bash
cd apps/logseq-capture && go run . send "todo Buy milk tomorrow"
```

Send shared commands:
```bash
cd apps/logseq-capture && go run . command "help"
cd apps/logseq-capture && go run . command "toggle also"
```

Review journals through the daemon:
```bash
cd apps/logseq-capture && go run . review today
cd apps/logseq-capture && go run . review yesterday
```

Start an interactive REPL:
```bash
cd apps/logseq-capture && go run . cli
```

The first CLI version is text-only. Telegram-specific photo and voice capture still works through the combined runtime and the Telegram-only adapter path.

## Usage

Send a message to your bot, or send text to the local daemon with the CLI. Both paths share the same parser, formatter, review logic, and session behavior.

### Profile Selection
- `/w [note]` or `/work [note]` - Captures to `work/journals/YYYY_MM_DD.md`.
- `/p [note]` or `/personal [note]` - Captures to `personal/journals/YYYY_MM_DD.md`.
- `[note]` (no prefix) - Defaults to `LOGSEQ_CAPTURE_DEFAULT_JOURNAL`, or **personal** if unset/invalid.

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

### Review
- `/today` or `logseq-capture review today`
- `/yesterday` or `logseq-capture review yesterday`

### Bot Feedback
- ✅ **Success**: The bot will reply with the profile used and the exact formatted entry.
- ❌ **Error**: If something goes wrong, the bot will reply with a detailed error message.

## Local HTTP API

The daemon exposes a localhost-only JSON API:
- `POST /capture`
- `POST /command`
- `GET /review?day=today|yesterday`
- `GET /health`

## Automation
For a permanent setup, it is reasonable to run either the Telegram runtime or the local daemon as a systemd service, depending on which adapter you want active by default.
