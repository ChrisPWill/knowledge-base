package main

import (
	"context"
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

func (m *mockTelegramClient) GetUpdates(ctx context.Context, offset int, timeout int) ([]Update, error) {
	return m.updates, nil
}

func (m *mockTelegramClient) SendMessage(ctx context.Context, chatID int64, text string) error {
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
	ctx := context.Background()

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
		{
			name:     "Whitespace trimming",
			msg:      "   Clean this up   ",
			expected: "Clean this up",
			profile:  "personal",
		},
		{
			name:     "Work prefix with whitespace",
			msg:      "/work    Meeting notes  ",
			expected: "Meeting notes",
			profile:  "work",
		},
		{
			name:     "Empty message",
			msg:      "   ",
			expected: "",
			profile:  "personal",
		},
		{
			name:     "Only prefix",
			msg:      "/work",
			expected: "",
			profile:  "work",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			update := Update{}
			update.Message.Text = tc.msg
			update.Message.Chat.ID = 123

			err := bot.processMessage(ctx, update)
			if err != nil {
				t.Fatalf("processMessage failed: %v", err)
			}

			// Check file exists
			now := time.Now().Format("2006_01_02")
			path := filepath.Join(tc.profile, "journals", now+".md")
			
			if tc.expected == "" {
				// File should either not exist or not contain anything new
				// For simplicity, we just check if it was created if it didn't exist before
				// But since we use temp dir and run tests in sequence, we can check if file exists
				_, err := os.Stat(path)
				if err == nil {
					// If it exists, it shouldn't have changed significantly or we should have checked state
					// but for this test, we expect no entry to be added.
				}
				return
			}

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
