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

// MessageMetadata tracks where a message was saved to allow for edits/deletes.
type MessageMetadata struct {
	FilePath  string    `json:"file_path"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
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
	mapFile      string
	rootDir      string
	titleFetcher func(ctx context.Context, url string) (string, error)
	service      *CaptureService
	media        *MediaService
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
	service := NewCaptureService(rootDir)
	service.SetTitleFetcher(fetcher)
	return &Bot{
		client:       client,
		offsetFile:   offsetFile,
		mapFile:      offsetFile + ".map",
		rootDir:      rootDir,
		service:      service,
		media:        NewMediaService(client, rootDir),
		titleFetcher: fetcher,
	}
}

func (b *Bot) Run(ctx context.Context) error {
	offset := 0
	if data, err := os.ReadFile(b.offsetFile); err == nil {
		fmt.Sscanf(string(data), "%d", &offset)
	}

	b.service.LoadMessageMap("telegram", b.mapFile)

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
				b.service.SaveMessageMap("telegram", b.mapFile)
			}

			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
			}
		}
	}
}

func (b *Bot) processMessage(ctx context.Context, update Update, now time.Time) error {
	b.service.SetRootDir(b.rootDir)
	b.service.SetTitleFetcher(b.titleFetcher)
	b.media.rootDir = b.rootDir

	msgObj := update.Message
	if msgObj == nil {
		msgObj = update.EditedMessage
	}

	req, isCommand := b.mapUpdateToRequest(update, now)
	downloader := AssetDownloader(b.media)
	if isCommand {
		downloader = nil
	}

	resp, err := b.service.Process(ctx, req, downloader)
	if err != nil {
		_ = b.client.SendMessage(ctx, msgObj.Chat.ID, fmt.Sprintf("❌ Error: %v", err))
		return err
	}
	if resp.Reply == "" {
		return nil
	}
	if resp.Entry != "" {
		slog.Info("Captured message", "profile", resp.Profile, "entry", strings.TrimSpace(resp.Entry))
	}
	return b.client.SendMessage(ctx, msgObj.Chat.ID, resp.Reply)
}

func (b *Bot) mapUpdateToRequest(update Update, now time.Time) (CaptureRequest, bool) {
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

	clientID := fmt.Sprintf("telegram:%d", msgObj.Chat.ID)
	text := strings.TrimSpace(msgObj.Text)

	if !isEdit && text != "" {
		lower := strings.ToLower(text)
		switch {
		case lower == "/today" || lower == "/yesterday":
			return CaptureRequest{
				ClientID:  clientID,
				Kind:      RequestKindReview,
				ReviewDay: strings.TrimPrefix(lower, "/"),
				Timestamp: &now,
			}, false
		case lower == "help" || strings.HasPrefix(lower, "help ") || lower == "toggle also":
			return CaptureRequest{
				ClientID:  clientID,
				Kind:      RequestKindCommand,
				Text:      text,
				Timestamp: &now,
			}, true
		}
	}

	return CaptureRequest{
		ClientID:  clientID,
		Kind:      RequestKindCapture,
		Text:      text,
		Timestamp: &now,
		Item:      &item,
	}, false
}
