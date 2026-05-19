package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	tests := []struct {
		name           string
		msg            string
		expected       string
		profile        string
		expectNoInbox  bool
		expectedFormat string // regex to match the whole line
	}{
		{
			name:           "Personal default",
			msg:            "Hello world",
			expected:       "Hello world #inbox",
			profile:        "personal",
			expectedFormat: `^- \d{2}:\d{2} Hello world #inbox\n$`,
		},
		{
			name:           "Work prefix",
			msg:            "/work Meeting notes",
			expected:       "Meeting notes #inbox",
			profile:        "work",
			expectedFormat: `^- \d{2}:\d{2} Meeting notes #inbox\n$`,
		},
		{
			name:           "TODO mixed case",
			msg:            "tOdO fix bug",
			expected:       "fix bug #inbox",
			profile:        "personal",
			expectedFormat: `^- TODO \d{2}:\d{2} fix bug #inbox\n$`,
		},
		{
			name:          "Message with existing tag",
			msg:           "Meeting with #team",
			expected:      "Meeting with #team",
			profile:       "personal",
			expectNoInbox: true,
		},
		{
			name:          "Tag at the beginning",
			msg:           "#idea new feature",
			expected:      "#idea new feature",
			profile:       "personal",
			expectNoInbox: true,
		},
		{
			name:          "Multiple tags",
			msg:           "Buy #milk and #bread",
			expected:      "Buy #milk and #bread",
			profile:       "personal",
			expectNoInbox: true,
		},
		{
			name:     "Empty message",
			msg:      "   ",
			expected: "",
			profile:  "personal",
		},
		{
			name:     "Help command",
			msg:      "help",
			expected: "help-sentinel", // Special value to skip file checks
			profile:  "personal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caseTmpDir, err := os.MkdirTemp("", "logseq-case-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(caseTmpDir)

			mock := &mockTelegramClient{}
			bot := NewBot(mock, filepath.Join(caseTmpDir, ".offset"))
			bot.rootDir = caseTmpDir
			ctx := context.Background()

			update := Update{}
			update.Message.Text = tc.msg
			update.Message.Chat.ID = 123

			err = bot.processMessage(ctx, update)
			if err != nil {
				t.Fatalf("processMessage failed: %v", err)
			}

			if tc.expected == "" {
				return
			}

			if tc.expected == "help-sentinel" {
				if len(mock.sent) != 1 || !strings.Contains(mock.sent[0], "Telegram Capture Help") {
					t.Errorf("expected help message, got %v", mock.sent)
				}
				return
			}

			// Check file exists
			now := time.Now().Format("2006_01_02")
			path := filepath.Join(caseTmpDir, tc.profile, "journals", now+".md")

			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("could not read journal file: %v", err)
			}

			if !strings.Contains(string(content), tc.expected) {
				t.Errorf("expected content %q not found in %q", tc.expected, string(content))
			}

			if tc.expectNoInbox && strings.Contains(string(content), "#inbox") {
				t.Errorf("did not expect #inbox in message %q, got: %q", tc.msg, string(content))
			}

			if tc.expectedFormat != "" {
				matched, err := regexp.MatchString(tc.expectedFormat, string(content))
				if err != nil {
					t.Fatal(err)
				}
				if !matched {
					t.Errorf("content %q does not match expected format %q", string(content), tc.expectedFormat)
				}
			}

			// Check confirmation message
			if len(mock.sent) != 1 {
				t.Errorf("expected 1 message, got %d", len(mock.sent))
				return
			}
			if !strings.Contains(mock.sent[0], "✅ Captured to "+tc.profile) {
				t.Errorf("expected success emoji and profile in %q", mock.sent[0])
			}
			if !strings.Contains(mock.sent[0], tc.expected) {
				t.Errorf("expected entry text %q in confirmation %q", tc.expected, mock.sent[0])
			}
		})
	}
}

func TestProcessMessageError(t *testing.T) {
	caseTmpDir, err := os.MkdirTemp("", "logseq-err-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(caseTmpDir)

	// Create a file where a directory should be to force MkdirAll to fail
	err = os.WriteFile(filepath.Join(caseTmpDir, "personal"), []byte("not a directory"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	mock := &mockTelegramClient{}
	bot := NewBot(mock, filepath.Join(caseTmpDir, ".offset"))
	bot.rootDir = caseTmpDir
	ctx := context.Background()

	update := Update{}
	update.Message.Text = "This should fail"
	update.Message.Chat.ID = 123

	err = bot.processMessage(ctx, update)
	if err == nil {
		t.Fatal("expected error but got nil")
	}

	if len(mock.sent) != 1 || !strings.Contains(mock.sent[0], "❌ Error:") {
		t.Errorf("expected error reply, got %v", mock.sent)
	}
}

func TestBotRun(t *testing.T) {
	caseTmpDir, err := os.MkdirTemp("", "logseq-run-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(caseTmpDir)

	offsetFile := filepath.Join(caseTmpDir, ".offset")

	mock := &mockTelegramClient{
		updates: []Update{
			{UpdateID: 100, Message: struct {
				Text string `json:"text"`
				Chat struct {
					ID int64 `json:"id"`
				} `json:"chat"`
			}{Text: "Message 1", Chat: struct{ ID int64 `json:"id"` }{ID: 1}}},
			{UpdateID: 101, Message: struct {
				Text string `json:"text"`
				Chat struct {
					ID int64 `json:"id"`
				} `json:"chat"`
			}{Text: "Message 2", Chat: struct{ ID int64 `json:"id"` }{ID: 1}}},
		},
	}

	bot := NewBot(mock, offsetFile)
	bot.rootDir = caseTmpDir

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run in background and stop after a short while
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	err = bot.Run(ctx)
	if err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("Bot.Run failed: %v", err)
	}

	// Verify offset was updated
	data, err := os.ReadFile(offsetFile)
	if err != nil {
		t.Fatalf("could not read offset file: %v", err)
	}
	var offset int
	fmt.Sscanf(string(data), "%d", &offset)
	if offset != 102 {
		t.Errorf("expected offset 102, got %d", offset)
	}

	// Verify both messages were captured
	now := time.Now().Format("2006_01_02")
	path := filepath.Join(caseTmpDir, "personal", "journals", now+".md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read journal file: %v", err)
	}

	if !strings.Contains(string(content), "Message 1") || !strings.Contains(string(content), "Message 2") {
		t.Errorf("messages not found in journal: %q", string(content))
	}
}
