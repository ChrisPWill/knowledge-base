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

type PhotoSize struct {
	FileID   string `json:"file_id"`
	FileSize int    `json:"file_size"`
}

type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Venue struct {
	Location Location `json:"location"`
	Title    string   `json:"title"`
	Address  string   `json:"address"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type Message struct {
	MessageID int64       `json:"message_id"`
	Text      string      `json:"text"`
	Chat      Chat        `json:"chat"`
	Photo     []PhotoSize `json:"photo"`
	Voice     *PhotoSize  `json:"voice"` // Voice also has file_id
	Location  *Location   `json:"location"`
	Venue     *Venue      `json:"venue"`
	Caption   string      `json:"caption"`
}

type Update struct {
	UpdateID      int      `json:"update_id"`
	Message       *Message `json:"message"`
	EditedMessage *Message `json:"edited_message"`
}

// MessageMetadata tracks where a message was saved to allow for edits/deletes
type MessageMetadata struct {
	FilePath    string    `json:"file_path"`
	Content     string    `json:"content"`      // The full line as written to the file
	Timestamp   time.Time `json:"timestamp"`    // When it was captured
}

type GetUpdatesResponse struct {
	OK     bool     `json:"ok"`
	Result []Update `json:"result"`
}

type TelegramClient interface {
	GetUpdates(ctx context.Context, offset int, timeout int) ([]Update, error)
	SendMessage(ctx context.Context, chatID int64, text string) error
	GetFile(ctx context.Context, fileID string) (string, error)
	DownloadFile(ctx context.Context, filePath string, destPath string) error
}

type httpTelegramClient struct {
	token      string
	apiURL     string
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

func (c *httpTelegramClient) GetFile(ctx context.Context, fileID string) (string, error) {
	u := fmt.Sprintf("%s/getFile?file_id=%s", c.apiURL, fileID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("telegram api returned status %d", resp.StatusCode)
	}

	var data struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if !data.OK {
		return "", fmt.Errorf("telegram api returned ok=false")
	}

	return data.Result.FilePath, nil
}

func (c *httpTelegramClient) DownloadFile(ctx context.Context, filePath string, destPath string) error {
	u := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", c.token, filePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file: status %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save file: %w", err)
	}

	return nil
}

type Bot struct {
	client       TelegramClient
	offsetFile   string
	rootDir      string
	parser       *MessageParser
	journal      *JournalStore
	formatter    *LogseqFormatter
	commands     *CommandDispatcher
	media        *MediaService
	state        SessionState
	messageMap   map[int64]MessageMetadata
	mapFile      string
	titleFetcher func(ctx context.Context, url string) (string, error)
}

func (b *Bot) loadMessageMap() {
	if data, err := os.ReadFile(b.mapFile); err == nil {
		json.Unmarshal(data, &b.messageMap)
	}
	if b.messageMap == nil {
		b.messageMap = make(map[int64]MessageMetadata)
	}
}

func (b *Bot) saveMessageMap() {
	now := time.Now()
	for id, meta := range b.messageMap {
		if now.Sub(meta.Timestamp) > 24*time.Hour {
			delete(b.messageMap, id)
		}
	}

	if data, err := json.Marshal(b.messageMap); err == nil {
		os.WriteFile(b.mapFile, data, 0644)
	}
}

func defaultTitleFetcher(ctx context.Context, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; LogseqBot/1.0)")

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10240))
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`(?i)<title>(.*?)</title>`)
	matches := re.FindStringSubmatch(string(body))
	if len(matches) < 2 {
		return "", fmt.Errorf("no title found")
	}

	return strings.TrimSpace(matches[1]), nil
}

func NewBot(client TelegramClient, offsetFile string) *Bot {
	rootDir := "."
	fetcher := defaultTitleFetcher
	return &Bot{
		client:       client,
		offsetFile:   offsetFile,
		mapFile:      offsetFile + ".map",
		rootDir:      rootDir,
		parser:       NewMessageParser(rootDir, fetcher),
		journal:      NewJournalStore(rootDir),
		formatter:    &LogseqFormatter{},
		commands:     NewCommandDispatcher(),
		media:        NewMediaService(client, rootDir),
		messageMap:   make(map[int64]MessageMetadata),
		titleFetcher: fetcher,
	}
}

func (b *Bot) Run(ctx context.Context) error {
	offset := 0
	if data, err := os.ReadFile(b.offsetFile); err == nil {
		fmt.Sscanf(string(data), "%d", &offset)
	}

	b.loadMessageMap()

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

			backoff = 1 * time.Second

			for _, update := range updates {
				msg := update.Message
				if msg == nil {
					msg = update.EditedMessage
				}

				if msg != nil {
					if msg.Text != "" || len(msg.Photo) > 0 || msg.Voice != nil || msg.Location != nil || msg.Venue != nil {
						if err := b.processMessage(ctx, update, time.Now()); err != nil {
							slog.Error("Error processing message", "update_id", update.UpdateID, "error", err)
						}
					}
				}
				offset = update.UpdateID + 1
				if err := os.WriteFile(b.offsetFile, []byte(fmt.Sprintf("%d", offset)), 0644); err != nil {
					slog.Error("Error saving offset", "offset", offset, "error", err)
				}
				b.saveMessageMap()
			}

			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
			}
		}
	}
}

func (b *Bot) processMessage(ctx context.Context, update Update, now time.Time) error {
	// Re-sync rootDir and titleFetcher to parser/journal if they were changed (for tests)
	b.parser.rootDir = b.rootDir
	b.parser.titleFetcher = b.titleFetcher
	b.parser.rules.rootDir = b.rootDir
	b.journal.rootDir = b.rootDir
	b.media.rootDir = b.rootDir

	// Handle toggle timeout
	if b.state.IsToggledAlso && now.Sub(b.state.LastInteractionTime) > 5*time.Minute {
		b.state.IsToggledAlso = false
	}

	if update.EditedMessage != nil {
		return b.handleEdit(ctx, update, now)
	}

	msgObj := update.Message
	msg := strings.TrimSpace(msgObj.Text)

	// 1. Dispatch Commands
	handled, err := b.commands.Dispatch(ctx, b, msgObj.Chat.ID, msg, now)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	// 2. Normal Capture
	entry, profile, err := b.handleMessage(ctx, update, now)
	if err != nil {
		errMsg := fmt.Sprintf("❌ Error: %v", err)
		b.client.SendMessage(ctx, msgObj.Chat.ID, errMsg)
		return err
	}

	if entry == "" {
		return nil
	}

	// Track metadata for future edits
	b.messageMap[msgObj.MessageID] = MessageMetadata{
		FilePath:  b.state.LastJournalFile,
		Content:   entry,
		Timestamp: now,
	}

	slog.Info("Captured message", "profile", profile, "entry", strings.TrimSpace(entry))

	confirmMsg := fmt.Sprintf("✅ Captured to %s journal:\n%s", profile, strings.TrimSuffix(entry, "\n"))
	return b.client.SendMessage(ctx, msgObj.Chat.ID, confirmMsg)
}

func (b *Bot) sendHelp(ctx context.Context, chatID int64, lowerMsg string) error {
	topic := strings.TrimSpace(strings.TrimPrefix(lowerMsg, "help"))
	var helpText string
	switch topic {
	case "nesting":
		helpText = "*Nesting Help*\n\n" +
			"• `also [note]` - Nest under the last entry\n" +
			"• `toggle also` - Enable auto-nesting mode (5m timeout)\n" +
			"• Auto-nesting is disabled if a profile prefix (/w, /p) is used."
	case "priority":
		helpText = "*Priority Help*\n\n" +
			"• `A [note]` - High priority ([#A])\n" +
			"• `B [note]` - Medium priority ([#B])\n" +
			"• `C [note]` - Low priority ([#C])\n" +
			"• Can be combined with `todo`: `todo A fix bug`"
	case "scheduling":
		helpText = "*Scheduling Help*\n\n" +
			"• `... scheduled for tomorrow` -> SCHEDULED: <YYYY-MM-DD Day>\n" +
			"• `... deadline next friday` -> DEADLINE: <YYYY-MM-DD Day>"
	case "media":
		helpText = "*Media Help*\n\n" +
			"• Photos and voice notes are downloaded to Logseq's `assets/` folder.\n" +
			"• Captions are supported and appended to the link."
	default:
		helpText = "🤖 *Telegram Capture Help*\n\n" +
			"*Profiles:*\n" +
			"• `/w [note]` or `/work [note]` - Work journal\n" +
			"• `/p [note]` or `/personal [note]` - Personal journal\n" +
			"• `[note]` - Defaults to personal\n\n" +
			"*Review:* `/today`, `/yesterday`\n" +
			"*Topics:* `help nesting`, `help priority`, `help scheduling`, `help media`"
	}

	return b.client.SendMessage(ctx, chatID, helpText)
}

func (b *Bot) sendReview(ctx context.Context, chatID int64, lowerMsg string, now time.Time) error {
	targetDate := now
	if lowerMsg == "/yesterday" {
		targetDate = now.AddDate(0, 0, -1)
	}

	responseText, err := b.journal.ReadReview(ctx, targetDate)
	if err != nil {
		return err
	}
	if responseText == "" {
		label := "today"
		if lowerMsg == "/yesterday" {
			label = "yesterday"
		}
		responseText = fmt.Sprintf("📭 No entries found for %s.", label)
	}

	return b.client.SendMessage(ctx, chatID, responseText)
}

func (b *Bot) toggleAlso(ctx context.Context, chatID int64, now time.Time) error {
	b.state.IsToggledAlso = !b.state.IsToggledAlso
	b.state.LastInteractionTime = now
	var confirm string
	if b.state.IsToggledAlso {
		confirm = "✅ Also mode enabled. Messages will be nested for 5 minutes of inactivity."
	} else {
		confirm = "❌ Also mode disabled."
	}
	return b.client.SendMessage(ctx, chatID, confirm)
}

func (b *Bot) handleMessage(ctx context.Context, update Update, now time.Time) (string, string, error) {
	item := b.mapUpdateToCaptureItem(ctx, update, now)

	// 1. Analyze message
	block, profile, err := b.parser.Analyze(ctx, item, b.state)
	if err != nil {
		return "", "", err
	}

	if block.Text == "" {
		return "", "", nil
	}

	// 2. Download media if present
	if item.HasMedia {
		if _, err := b.media.DownloadAsset(ctx, &item, profile); err != nil {
			return "", "", err
		}
	}

	// 3. Format entry
	entry := b.formatter.Format(block, b.state)

	// If we are editing, don't write to file here; handleEdit will do it.
	if item.IsEdit {
		return entry, profile, nil
	}

	// 4. Persist
	filePath, err := b.journal.Append(profile, entry, item.Timestamp)
	if err != nil {
		return "", "", err
	}

	// 5. Update state for nesting
	if block.IndentLevel == 0 {
		b.state.LastProfile = profile
		b.state.LastJournalFile = filePath
	}
	b.state.LastEntryTime = item.Timestamp
	b.state.LastInteractionTime = now

	return entry, profile, nil
}

func (b *Bot) handleEdit(ctx context.Context, update Update, now time.Time) error {
	msgObj := update.EditedMessage
	meta, ok := b.messageMap[msgObj.MessageID]
	if !ok {
		return fmt.Errorf("no metadata found for message %d", msgObj.MessageID)
	}

	if now.Sub(meta.Timestamp) > 1*time.Hour {
		return fmt.Errorf("edit window expired for message %d", msgObj.MessageID)
	}

	newEntry, _, err := b.handleMessage(ctx, update, meta.Timestamp)
	if err != nil {
		return err
	}

	if err := b.journal.Update(meta.FilePath, meta.Content, newEntry); err != nil {
		return err
	}

	if newEntry == "" {
		delete(b.messageMap, msgObj.MessageID)
	} else {
		meta.Content = newEntry
		b.messageMap[msgObj.MessageID] = meta
	}

	return b.client.SendMessage(ctx, msgObj.Chat.ID, "🔄 Updated entry in journal.")
}

func (b *Bot) mapUpdateToCaptureItem(ctx context.Context, update Update, now time.Time) CaptureItem {
	msgObj := update.Message
	isEdit := false
	if msgObj == nil {
		msgObj = update.EditedMessage
		isEdit = true
	}

	item := CaptureItem{
		ID:        msgObj.MessageID,
		Text:      msgObj.Text,
		Caption:   msgObj.Caption,
		Timestamp: now,
		IsEdit:    isEdit,
	}

	if msgObj.Location != nil {
		item.HasLocation = true
		item.Latitude = msgObj.Location.Latitude
		item.Longitude = msgObj.Location.Longitude
	}
	if msgObj.Venue != nil {
		item.HasLocation = true
		item.Latitude = msgObj.Venue.Location.Latitude
		item.Longitude = msgObj.Venue.Location.Longitude
		item.VenueTitle = msgObj.Venue.Title
		item.VenueAddr = msgObj.Venue.Address
	}
	if len(msgObj.Photo) > 0 {
		item.HasMedia = true
		item.MediaID = msgObj.Photo[len(msgObj.Photo)-1].FileID
	}
	if msgObj.Voice != nil {
		item.HasMedia = true
		item.IsVoice = true
		item.MediaID = msgObj.Voice.FileID
	}

	return item
}
