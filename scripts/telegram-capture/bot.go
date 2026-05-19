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

	"github.com/tj/go-naturaldate"
)

var tagRegex = regexp.MustCompile(`(^|\s)#\S+`)
var urlRegex = regexp.MustCompile(`https?://[^\s]+`)

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
	client              TelegramClient
	offsetFile          string
	rootDir             string
	lastProfile         string
	lastJournalFile     string
	lastEntryTime       time.Time
	isToggledAlso       bool
	lastInteractionTime time.Time
	titleFetcher        func(ctx context.Context, url string) (string, error)
	messageMap          map[int64]MessageMetadata
	mapFile             string
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
	// Cleanup old entries (> 24h) to keep it lean
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
	// Mimic a real browser to avoid some bot blocks
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

	// Read first 10KB to find title
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
	return &Bot{
		client:       client,
		offsetFile:   offsetFile,
		mapFile:      offsetFile + ".map",
		rootDir:      ".",
		titleFetcher: defaultTitleFetcher,
		messageMap:   make(map[int64]MessageMetadata),
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

			// Reset backoff on success
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

			// Small sleep to avoid tight loop if GetUpdates returns immediately
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
			}
		}
	}
}

func (b *Bot) processMessage(ctx context.Context, update Update, now time.Time) error {
	if update.EditedMessage != nil {
		if err := b.handleEdit(ctx, update, now); err != nil {
			slog.Error("Failed to handle edit", "error", err)
		}
		return nil
	}

	msgObj := update.Message
	msg := strings.TrimSpace(msgObj.Text)
	lowerMsg := strings.ToLower(msg)

	if strings.HasPrefix(lowerMsg, "help") {
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

		if err := b.client.SendMessage(ctx, update.Message.Chat.ID, helpText); err != nil {
			slog.Warn("Failed to send help message", "error", err)
		}
		return nil
	}

	if lowerMsg == "/today" || lowerMsg == "/yesterday" {
		targetDate := now
		label := "today"
		if lowerMsg == "/yesterday" {
			targetDate = now.AddDate(0, 0, -1)
			label = "yesterday"
		}

		// Check both profiles
		var reviewText strings.Builder
		for _, profile := range []string{"personal", "work"} {
			dateStr := targetDate.Format("2006_01_02")
			journalFile := filepath.Join(b.rootDir, profile, "journals", dateStr+".md")

			if content, err := os.ReadFile(journalFile); err == nil {
				if reviewText.Len() > 0 {
					reviewText.WriteString("\n")
				}
				profileLabel := profile
				if len(profileLabel) > 0 {
					profileLabel = strings.ToUpper(profileLabel[:1]) + profileLabel[1:]
				}
				reviewText.WriteString(fmt.Sprintf("📖 *%s (%s)*\n", profileLabel, label))
				reviewText.WriteString(string(content))
			}
		}

		responseText := reviewText.String()
		if responseText == "" {
			responseText = fmt.Sprintf("📭 No entries found for %s.", label)
		}

		if err := b.client.SendMessage(ctx, update.Message.Chat.ID, responseText); err != nil {
			slog.Warn("Failed to send review message", "error", err)
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

	entry, profile, err := b.handleMessage(ctx, update, now)
	if err != nil {
		errMsg := fmt.Sprintf("❌ Error: %v", err)
		if sendErr := b.client.SendMessage(ctx, msgObj.Chat.ID, errMsg); sendErr != nil {
			slog.Warn("Failed to send error message", "error", sendErr)
		}
		return err
	}

	if entry == "" {
		return nil
	}

	// Track metadata for future edits
	b.messageMap[msgObj.MessageID] = MessageMetadata{
		FilePath:  b.lastJournalFile,
		Content:   entry,
		Timestamp: now,
	}

	slog.Info("Captured message", "profile", profile, "entry", strings.TrimSpace(entry))

	confirmMsg := fmt.Sprintf("✅ Captured to %s journal:\n%s", profile, strings.TrimSuffix(entry, "\n"))
	if err := b.client.SendMessage(ctx, msgObj.Chat.ID, confirmMsg); err != nil {
		slog.Warn("Failed to send confirmation message", "error", err)
	}

	return nil
}

func (b *Bot) handleEdit(ctx context.Context, update Update, now time.Time) error {
	msgObj := update.EditedMessage
	meta, ok := b.messageMap[msgObj.MessageID]
	if !ok {
		return fmt.Errorf("no metadata found for message %d", msgObj.MessageID)
	}

	// Only allow edits within 1 hour for safety
	if now.Sub(meta.Timestamp) > 1*time.Hour {
		return fmt.Errorf("edit window expired for message %d", msgObj.MessageID)
	}

	// Generate the new entry
	// We use the original timestamp for consistency in formatting (e.g. 15:04)
	newEntry, _, err := b.handleMessage(ctx, update, meta.Timestamp)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(meta.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read journal file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		// Exact match of the whole line (including newline if it was there)
		if line+"\n" == meta.Content {
			if newEntry == "" {
				// Delete line
				lines = append(lines[:i], lines[i+1:]...)
			} else {
				// Update line
				lines[i] = strings.TrimSuffix(newEntry, "\n")
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("could not find original entry in %s", meta.FilePath)
	}

	newFileContent := strings.Join(lines, "\n")
	if err := os.WriteFile(meta.FilePath, []byte(newFileContent), 0644); err != nil {
		return fmt.Errorf("failed to update journal file: %w", err)
	}

	// Update metadata
	if newEntry == "" {
		delete(b.messageMap, msgObj.MessageID)
	} else {
		meta.Content = newEntry
		b.messageMap[msgObj.MessageID] = meta
	}

	if err := b.client.SendMessage(ctx, msgObj.Chat.ID, "🔄 Updated entry in journal."); err != nil {
		slog.Warn("Failed to send update confirmation", "error", err)
	}

	return nil
}

func (b *Bot) handleMessage(ctx context.Context, update Update, now time.Time) (string, string, error) {
	msgObj := update.Message
	if msgObj == nil {
		msgObj = update.EditedMessage
	}

	msg := ""
	if msgObj != nil {
		msg = strings.TrimSpace(msgObj.Text)
		if msg == "" && msgObj.Caption != "" {
			msg = strings.TrimSpace(msgObj.Caption)
		}
	}

	isLocation := msgObj != nil && (msgObj.Location != nil || msgObj.Venue != nil)

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
	} else if isLocation && now.Sub(b.lastInteractionTime) < 1*time.Minute {
		// Auto-nest locations sent within 1 minute of previous message
		isAlso = true
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
	}

	// Handle Location/Venue
	if isLocation {
		var lat, lon float64
		var locText string
		var u string

		if msgObj.Venue != nil {
			lat = msgObj.Venue.Location.Latitude
			lon = msgObj.Venue.Location.Longitude
			title := msgObj.Venue.Title
			address := msgObj.Venue.Address
			// Clean up address (often contains title or redundant info)
			address = strings.Split(address, ",")[0]
			u = fmt.Sprintf("https://www.google.com/maps?q=%f,%f", lat, lon)
			locText = fmt.Sprintf("%s: %s (%s)", title, address, u)
		} else {
			lat = msgObj.Location.Latitude
			lon = msgObj.Location.Longitude
			locText = fmt.Sprintf("https://www.google.com/maps?q=%f,%f", lat, lon)
		}

		if cleanMsg != "" {
			cleanMsg = locText + " " + cleanMsg
		} else {
			cleanMsg = locText
		}
	}

	// Handle media
	if msgObj != nil && (len(msgObj.Photo) > 0 || msgObj.Voice != nil) {
		var fileID string
		var extension string
		var format string

		if len(msgObj.Photo) > 0 {
			// Take the last photo (usually the largest)
			fileID = msgObj.Photo[len(msgObj.Photo)-1].FileID
			extension = ".jpg"
			format = "![Image](assets/%s)"
		} else {
			fileID = msgObj.Voice.FileID
			extension = ".ogg"
			format = "[Voice Note](assets/%s)"
		}

		filePath, err := b.client.GetFile(ctx, fileID)
		if err != nil {
			return "", "", fmt.Errorf("failed to get file path: %w", err)
		}

		assetName := fmt.Sprintf("capture_%s%s", now.Format("20060102_150405"), extension)
		assetDir := filepath.Join(b.rootDir, profile, "assets")
		assetPath := filepath.Join(assetDir, assetName)

		if err := b.client.DownloadFile(ctx, filePath, assetPath); err != nil {
			return "", "", fmt.Errorf("failed to download asset: %w", err)
		}

		mediaLink := fmt.Sprintf(format, assetName)
		if cleanMsg != "" {
			cleanMsg = mediaLink + " " + cleanMsg
		} else {
			cleanMsg = mediaLink
		}
	}

	if cleanMsg == "" {
		slog.Debug("Ignoring empty message", "update_id", update.UpdateID)
		return "", "", nil
	}

	if journalFile == "" {
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

	priority := ""
	if len(cleanMsg) >= 2 {
		firstTwo := strings.ToUpper(cleanMsg[:2])
		if firstTwo == "A " || firstTwo == "B " || firstTwo == "C " {
			priority = "[#" + string(firstTwo[0]) + "] "
			cleanMsg = strings.TrimSpace(cleanMsg[2:])
		}
	}

	// URL scraping
	urls := urlRegex.FindAllString(cleanMsg, -1)
	for _, u := range urls {
		title, err := b.titleFetcher(ctx, u)
		if err == nil && title != "" {
			cleanMsg = strings.ReplaceAll(cleanMsg, u, fmt.Sprintf("[%s](%s)", title, u))
		}
	}

	// Dynamic Tag Mapping from "personal/pages/Telegram Rules.md"
	// Expected format:
	// - #tag: keyword1, keyword2
	rulesFile := filepath.Join(b.rootDir, "personal", "pages", "Telegram Rules.md")
	if rulesContent, err := os.ReadFile(rulesFile); err == nil {
		lines := strings.Split(string(rulesContent), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "- #") {
				continue
			}
			parts := strings.SplitN(line[2:], ":", 2)
			if len(parts) != 2 {
				continue
			}
			tag := strings.TrimSpace(parts[0])
			if !strings.HasPrefix(tag, "#") {
				tag = "#" + tag
			}
			keywords := strings.Split(parts[1], ",")
			for _, kw := range keywords {
				kw = strings.TrimSpace(kw)
				if kw == "" {
					continue
				}
				// Case-insensitive match for whole word
				re := regexp.MustCompile("(?i)\\b" + regexp.QuoteMeta(kw) + "\\b")
				if re.MatchString(cleanMsg) {
					if !strings.Contains(cleanMsg, tag) {
						cleanMsg += " " + tag
					}
					break
				}
			}
		}
	}

	// Natural Language Scheduling
	var scheduleMarker string
	triggers := []struct {
		prefix string
		marker string
	}{
		{"scheduled for ", "SCHEDULED"},
		{"deadline ", "DEADLINE"},
	}

	// If it's a TODO, assume certain words imply a deadline
	if isTodo {
		for _, word := range []string{"today", "tomorrow", "next "} {
			triggers = append(triggers, struct {
				prefix string
				marker string
			}{word, "DEADLINE"})
		}
	}

	for _, trigger := range triggers {
		lower := strings.ToLower(cleanMsg)
		idx := strings.Index(lower, trigger.prefix)
		if idx != -1 {
			dateStr := cleanMsg[idx+len(trigger.prefix):]
			// Special case for triggers that are the whole word (today, tomorrow)
			// we want to parse the trigger word itself.
			if trigger.prefix == "today" || trigger.prefix == "tomorrow" {
				dateStr = trigger.prefix
			}

			parsedDate, err := naturaldate.Parse(dateStr, now)
			if err == nil {
				// Adjustment for "next " to ensure it's in the future
				if trigger.prefix == "next " && parsedDate.Before(now) {
					parsedDate = parsedDate.AddDate(0, 0, 7)
				}

				scheduleMarker = fmt.Sprintf(" %s: <%s %s>",
					trigger.marker,
					parsedDate.Format("2006-01-02"),
					parsedDate.Format("Mon"))

				cleanMsg = strings.TrimSpace(cleanMsg[:idx])
				break
			}
		}
	}

	tagSuffix := ""
	if !isAlso && !tagRegex.MatchString(cleanMsg) {
		tagSuffix = " #inbox"
	}

	timeStr := now.Format("15:04")
	var entry string

	if isAlso && b.lastJournalFile != "" {
		indent := "  "
		if now.Sub(b.lastEntryTime) > time.Hour {
			entry = fmt.Sprintf("%s- %s%s %s%s%s\n", indent, priority, timeStr, cleanMsg, scheduleMarker, tagSuffix)
		} else {
			entry = fmt.Sprintf("%s- %s%s%s%s\n", indent, priority, cleanMsg, scheduleMarker, tagSuffix)
		}
	} else {
		todoPrefix := ""
		if isTodo {
			todoPrefix = "TODO "
		}
		entry = fmt.Sprintf("- %s%s%s %s%s%s\n", todoPrefix, priority, timeStr, cleanMsg, scheduleMarker, tagSuffix)
	}

	// If we are editing, don't write to file here; handleEdit will do it.
	if update.EditedMessage != nil {
		return entry, profile, nil
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
