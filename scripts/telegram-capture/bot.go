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
	"strings"
	"time"
)

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
	client     TelegramClient
	offsetFile string
}

func NewBot(client TelegramClient, offsetFile string) *Bot {
	return &Bot{
		client:     client,
		offsetFile: offsetFile,
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
	profile := "personal"
	cleanMsg := msg

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
		return nil
	}

	now := time.Now()
	dateStr := now.Format("2006_01_02")
	timeStr := now.Format("15:04")

	journalDir := filepath.Join(profile, "journals")
	journalFile := filepath.Join(journalDir, dateStr+".md")

	if err := os.MkdirAll(journalDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", journalDir, err)
	}

	entry := fmt.Sprintf("- %s %s\n", timeStr, cleanMsg)
	f, err := os.OpenFile(journalFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", journalFile, err)
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("failed to write to file %s: %w", journalFile, err)
	}

	slog.Info("Captured message", "profile", profile, "message", cleanMsg)
	
	if err := b.client.SendMessage(ctx, update.Message.Chat.ID, fmt.Sprintf("Captured to %s journal.", profile)); err != nil {
		slog.Warn("Failed to send confirmation message", "error", err)
	}
	
	return nil
}
