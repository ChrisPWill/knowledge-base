package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mockTelegramClient struct {
	updates []Update
	sent    []string
}

func (m *mockTelegramClient) GetUpdates(offset int, timeout int) ([]Update, error) {
	return m.updates, nil
}

func (m *mockTelegramClient) SendMessage(chatID int64, text string) error {
	m.sent = append(m.sent, text)
	return nil
}

func TestProcessMessage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logseq-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Change working directory to tmpDir for test
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	mock := &mockTelegramClient{}
	bot := NewBot(mock, ".offset-test")

	tests := []struct {
		name     string
		msg      string
		expected string
		profile  string
	}{
		{
			name:     "Personal default",
			msg:      "Hello world",
			expected: "Hello world",
			profile:  "personal",
		},
		{
			name:     "Work prefix",
			msg:      "/work Meeting notes",
			expected: "Meeting notes",
			profile:  "work",
		},
		{
			name:     "Personal prefix",
			msg:      "/p Buy milk",
			expected: "Buy milk",
			profile:  "personal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			update := Update{}
			update.Message.Text = tc.msg
			update.Message.Chat.ID = 123

			bot.processMessage(update)

			// Check file exists
			now := time.Now().Format("2006_01_02")
			path := filepath.Join(tc.profile, "journals", now+".md")
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("could not read journal file: %v", err)
			}

			if !strings.Contains(string(content), tc.expected) {
				t.Errorf("expected content %q not found in %s", tc.expected, string(content))
			}
		})
	}
}
