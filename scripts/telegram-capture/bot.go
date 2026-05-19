package main

import (
	"encoding/json"
	"fmt"
	"io"
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
	GetUpdates(offset int, timeout int) ([]Update, error)
	SendMessage(chatID int64, text string) error
}

type httpTelegramClient struct {
	token  string
	apiURL string
}

func NewHTTPTelegramClient(token string) *httpTelegramClient {
	return &httpTelegramClient{
		token:  token,
		apiURL: fmt.Sprintf("https://api.telegram.org/bot%s", token),
	}
}

func (c *httpTelegramClient) GetUpdates(offset int, timeout int) ([]Update, error) {
	u := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=%d", c.apiURL, offset, timeout)
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram api returned status %d", resp.StatusCode)
	}

	var data GetUpdatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if !data.OK {
		return nil, fmt.Errorf("telegram api returned ok=false")
	}

	return data.Result, nil
}

func (c *httpTelegramClient) SendMessage(chatID int64, text string) error {
	u := fmt.Sprintf("%s/sendMessage", c.apiURL)
	formData := url.Values{}
	formData.Set("chat_id", fmt.Sprintf("%d", chatID))
	formData.Set("text", text)

	resp, err := http.PostForm(u, formData)
	if err != nil {
		return err
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

func (b *Bot) Run() error {
	offset := 0
	if data, err := os.ReadFile(b.offsetFile); err == nil {
		fmt.Sscanf(string(data), "%d", &offset)
	}

	fmt.Println("Starting Telegram capture bot (Go version)...")

	for {
		updates, err := b.client.GetUpdates(offset, 60)
		if err != nil {
			fmt.Printf("Error getting updates: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, update := range updates {
			if update.Message.Text != "" {
				b.processMessage(update)
			}
			offset = update.UpdateID + 1
			os.WriteFile(b.offsetFile, []byte(fmt.Sprintf("%d", offset)), 0644)
		}
		time.Sleep(1 * time.Second)
	}
}

func (b *Bot) processMessage(update Update) {
	msg := update.Message.Text
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

	now := time.Now()
	dateStr := now.Format("2006_01_02")
	timeStr := now.Format("15:04")

	journalDir := filepath.Join(profile, "journals")
	journalFile := filepath.Join(journalDir, dateStr+".md")

	if err := os.MkdirAll(journalDir, 0755); err != nil {
		fmt.Printf("Error creating directory %s: %v\n", journalDir, err)
		return
	}

	entry := fmt.Sprintf("- %s %s\n", timeStr, cleanMsg)
	f, err := os.OpenFile(journalFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error opening file %s: %v\n", journalFile, err)
		return
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		fmt.Printf("Error writing to file %s: %v\n", journalFile, err)
		return
	}

	fmt.Printf("Captured to %s journal: %s\n", profile, cleanMsg)
	b.client.SendMessage(update.Message.Chat.ID, fmt.Sprintf("Captured to %s journal.", profile))
}
