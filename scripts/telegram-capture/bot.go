package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var tagRegex = regexp.MustCompile(`(^|\s)#\S+`)

type Update struct {
	UpdateID int `json:"update_id"`
	Message  struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

type GetUpdatesResponse struct {
	OK     bool     `json:"ok"`
	Result []Update `json:"result"`
}

type TelegramClient interface {
	GetUpdates(ctx context.Context, offset int, timeout int) ([]Update, error)
	SendMessage(ctx context.Context, chatID int64, text string) error
}

type httpTelegramClient struct {
	token  string
	apiURL string
	httpClient *http.Client
}

func NewHTTPTelegramClient(token string) *httpTelegramClient {
	return &httpTelegramClient{
		token:  token,
		apiURL: fmt.Sprintf("https://api.telegram.org/bot%s", token),
		httpClient: &http.Client{
			Timeout: 70 * time.Second, // Slightly longer than the long-polling timeout
		},
	}
}

func (c *httpTelegramClient) GetUpdates(ctx context.Context, offset int, timeout int) ([]Update, error) {
	u := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=%d", c.apiURL, offset, timeout)
	
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram api returned status %d", resp.StatusCode)
	}

	var data GetUpdatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !data.OK {
		return nil, fmt.Errorf("telegram api returned ok=false")
	}

	return data.Result, nil
}

func (c *httpTelegramClient) SendMessage(ctx context.Context, chatID int64, text string) error {
	u := fmt.Sprintf("%s/sendMessage", c.apiURL)
	formData := url.Values{}
	formData.Set("chat_id", fmt.Sprintf("%d", chatID))
	formData.Set("text", text)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram api returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

type Bot struct {
	client              TelegramClient
	offsetFile          string
	rootDir             string
	lastProfile         string
	lastJournalFile     string
	lastEntryTime       time.Time
	isToggledAlso       bool
	lastInteractionTime time.Time
}

func NewBot(client TelegramClient, offsetFile string) *Bot {
	return &Bot{
		client:     client,
		offsetFile: offsetFile,
		rootDir:    ".",
	}
}

func (b *Bot) Run(ctx context.Context) error {
	offset := 0
	if data, err := os.ReadFile(b.offsetFile); err == nil {
		fmt.Sscanf(string(data), "%d", &offset)
	}

	slog.Info("Starting Telegram capture bot (Go version)...")

	backoff := 1 * time.Second
	maxBackoff := 1 * time.Minute

	for {
		select {
		case <-ctx.Done():
			slog.Info("Bot shutting down...")
			return ctx.Err()
		default:
			updates, err := b.client.GetUpdates(ctx, offset, 60)
			if err != nil {
				if ctx.Err() != nil {
					continue
				}
				slog.Error("Error getting updates", "error", err, "retry_in", backoff)
				
				select {
				case <-time.After(backoff):
					backoff *= 2
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
				case <-ctx.Done():
					continue
				}
				continue
			}

			// Reset backoff on success
			backoff = 1 * time.Second

			for _, update := range updates {
				if update.Message.Text != "" {
					if err := b.processMessage(ctx, update); err != nil {
						slog.Error("Error processing message", "update_id", update.UpdateID, "error", err)
					}
				}
				offset = update.UpdateID + 1
				if err := os.WriteFile(b.offsetFile, []byte(fmt.Sprintf("%d", offset)), 0644); err != nil {
					slog.Error("Error saving offset", "offset", offset, "error", err)
				}
			}
			
			// Small sleep to avoid tight loop if GetUpdates returns immediately
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
			}
		}
	}
}

func (b *Bot) processMessage(ctx context.Context, update Update) error {
	msg := strings.TrimSpace(update.Message.Text)
	lowerMsg := strings.ToLower(msg)
	now := time.Now()

	if lowerMsg == "help" {
		helpText := "🤖 *Telegram Capture Help*\n\n" +
			"*Profiles:*\n" +
			"• `/w [note]` or `/work [note]` - Work journal\n" +
			"• `/p [note]` or `/personal [note]` - Personal journal\n" +
			"• `[note]` - Defaults to personal\n\n" +
			"*Nesting:* \n" +
			"• `also [note]` - Nest note under the last entry\n" +
			"• `toggle also` - Enable auto-nesting mode (5m timeout)\n\n" +
			"*Features:*\n" +
			"• `todo [note]` - Captures as a Logseq TODO\n" +
			"• Automatic `#inbox` tag added if no tags are present (top-level only)\n\n" +
			"*Example:*\n" +
			"`todo Buy milk #shopping`"
		if err := b.client.SendMessage(ctx, update.Message.Chat.ID, helpText); err != nil {
			slog.Warn("Failed to send help message", "error", err)
		}
		return nil
	}

	if lowerMsg == "toggle also" {
		b.isToggledAlso = !b.isToggledAlso
		b.lastInteractionTime = now
		var confirm string
		if b.isToggledAlso {
			confirm = "✅ Also mode enabled. Messages will be nested for 5 minutes of inactivity."
		} else {
			confirm = "❌ Also mode disabled."
		}
		if err := b.client.SendMessage(ctx, update.Message.Chat.ID, confirm); err != nil {
			slog.Warn("Failed to send toggle confirmation", "error", err)
		}
		return nil
	}

	entry, profile, err := b.handleMessage(ctx, update)
	if err != nil {
		errMsg := fmt.Sprintf("❌ Error: %v", err)
		if sendErr := b.client.SendMessage(ctx, update.Message.Chat.ID, errMsg); sendErr != nil {
			slog.Warn("Failed to send error message", "error", sendErr)
		}
		return err
	}

	if entry == "" {
		return nil
	}

	slog.Info("Captured message", "profile", profile, "entry", strings.TrimSpace(entry))

	confirmMsg := fmt.Sprintf("✅ Captured to %s journal:\n%s", profile, strings.TrimSuffix(entry, "\n"))
	if err := b.client.SendMessage(ctx, update.Message.Chat.ID, confirmMsg); err != nil {
		slog.Warn("Failed to send confirmation message", "error", err)
	}

	return nil
}

func (b *Bot) handleMessage(ctx context.Context, update Update) (string, string, error) {
	msg := strings.TrimSpace(update.Message.Text)
	now := time.Now()

	// Handle toggle timeout
	if b.isToggledAlso && now.Sub(b.lastInteractionTime) > 5*time.Minute {
		b.isToggledAlso = false
	}

	// Check for "also " prefix or toggle mode
	isAlso := false
	if strings.HasPrefix(strings.ToLower(msg), "also ") {
		isAlso = true
		msg = strings.TrimSpace(msg[len("also "):])
	} else if b.isToggledAlso {
		// Only auto-nest if it doesn't have a profile prefix
		hasPrefix := strings.HasPrefix(msg, "/w ") || strings.HasPrefix(msg, "/work ") ||
			strings.HasPrefix(msg, "/p ") || strings.HasPrefix(msg, "/personal ")
		if !hasPrefix {
			isAlso = true
		}
	}

	var profile string
	var cleanMsg string
	var journalFile string
	var journalDir string
	var dateStr string

	if isAlso && b.lastJournalFile != "" {
		// Use previous state for nesting
		profile = b.lastProfile
		journalFile = b.lastJournalFile
		cleanMsg = msg
	} else {
		// Normal top-level capture
		profile = "personal"
		cleanMsg = msg

		if strings.HasPrefix(msg, "/w ") || strings.HasPrefix(msg, "/work ") {
			profile = "work"
			cleanMsg = strings.TrimPrefix(msg, "/w ")
			cleanMsg = strings.TrimPrefix(cleanMsg, "/work ")
		} else if strings.HasPrefix(msg, "/p ") || strings.HasPrefix(msg, "/personal ") {
			profile = "personal"
			cleanMsg = strings.TrimPrefix(msg, "/p ")
			cleanMsg = strings.TrimPrefix(cleanMsg, "/personal ")
		}

		cleanMsg = strings.TrimSpace(cleanMsg)
		if cleanMsg == "" {
			slog.Debug("Ignoring empty message", "update_id", update.UpdateID)
			return "", "", nil
		}

		dateStr = now.Format("2006_01_02")
		journalDir = filepath.Join(b.rootDir, profile, "journals")
		journalFile = filepath.Join(journalDir, dateStr+".md")

		if err := os.MkdirAll(journalDir, 0755); err != nil {
			return "", "", fmt.Errorf("failed to create directory %s: %w", journalDir, err)
		}
	}

	isTodo := false
	if strings.HasPrefix(strings.ToLower(cleanMsg), "todo ") {
		isTodo = true
		cleanMsg = strings.TrimSpace(cleanMsg[len("todo "):])
	}

	// Only default tag top-level notes
	if !isAlso && !tagRegex.MatchString(cleanMsg) {
		cleanMsg = cleanMsg + " #inbox"
	}

	timeStr := now.Format("15:04")
	var entry string

	if isAlso && b.lastJournalFile != "" {
		indent := "  "
		if now.Sub(b.lastEntryTime) > time.Hour {
			entry = fmt.Sprintf("%s- %s %s\n", indent, timeStr, cleanMsg)
		} else {
			entry = fmt.Sprintf("%s- %s\n", indent, cleanMsg)
		}
	} else {
		if isTodo {
			entry = fmt.Sprintf("- TODO %s %s\n", timeStr, cleanMsg)
		} else {
			entry = fmt.Sprintf("- %s %s\n", timeStr, cleanMsg)
		}
	}

	f, err := os.OpenFile(journalFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return "", "", fmt.Errorf("failed to open file %s: %w", journalFile, err)
	}
	defer f.Close()

	// Check for Logseq's default "-" stub in new files
	info, err := f.Stat()
	if err == nil && info.Size() > 0 {
		content, err := io.ReadAll(f)
		if err == nil {
			trimmed := strings.TrimSpace(string(content))
			if trimmed == "-" || trimmed == "" {
				// It's a stub or empty, clear it
				f.Truncate(0)
				f.Seek(0, 0)
			} else {
				// Not a stub, move to end and ensure we start on a new line
				f.Seek(0, 2)
				if !strings.HasSuffix(string(content), "\n") {
					f.WriteString("\n")
				}
			}
		} else {
			f.Seek(0, 2)
		}
	}

	if _, err := f.WriteString(entry); err != nil {
		return "", "", fmt.Errorf("failed to write to file %s: %w", journalFile, err)
	}

	// Update state
	if !isAlso {
		b.lastProfile = profile
		b.lastJournalFile = journalFile
	}
	b.lastEntryTime = now
	b.lastInteractionTime = now

	return entry, profile, nil
}
