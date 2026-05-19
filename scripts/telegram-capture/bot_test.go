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
		{
			name:           "Priority A",
			msg:            "A critical bug",
			expected:       "[#A]",
			profile:        "personal",
			expectedFormat: `^- \[#A\] \d{2}:\d{2} critical bug #inbox\n$`,
		},
		{
			name:           "Priority B with TODO",
			msg:            "todo B follow up",
			expected:       "TODO [#B]",
			profile:        "personal",
			expectedFormat: `^- TODO \[#B\] \d{2}:\d{2} follow up #inbox\n$`,
		},
		{
			name:           "Priority C with Profile and TODO",
			msg:            "/work todo C minor task",
			expected:       "TODO [#C]",
			profile:        "work",
			expectedFormat: `^- TODO \[#C\] \d{2}:\d{2} minor task #inbox\n$`,
		},
		{
			name:           "URL with Title",
			msg:            "Check https://example.com",
			expected:       "Check [Example](https://example.com)",
			profile:        "personal",
			expectedFormat: `^- \d{2}:\d{2} Check \[Example\]\(https://example.com\) #inbox\n$`,
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
			bot.titleFetcher = func(ctx context.Context, u string) (string, error) {
				if u == "https://example.com" {
					return "Example", nil
				}
				return "", fmt.Errorf("not found")
			}
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

func TestNesting(t *testing.T) {
	caseTmpDir, err := os.MkdirTemp("", "logseq-nesting-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(caseTmpDir)

	mock := &mockTelegramClient{}
	bot := NewBot(mock, filepath.Join(caseTmpDir, ".offset"))
	bot.rootDir = caseTmpDir
	ctx := context.Background()

	// 1. Send a parent note
	update1 := Update{}
	update1.Message.Text = "Parent Note"
	update1.Message.Chat.ID = 123
	bot.processMessage(ctx, update1)

	// 2. Send an "also" note (within 1 hour)
	update2 := Update{}
	update2.Message.Text = "also Child 1"
	update2.Message.Chat.ID = 123
	bot.processMessage(ctx, update2)

	// 3. Send another "also" note (fake time to > 1 hour)
	bot.lastEntryTime = bot.lastEntryTime.Add(-2 * time.Hour)
	update3 := Update{}
	update3.Message.Text = "also Child 2"
	update3.Message.Chat.ID = 123
	bot.processMessage(ctx, update3)

	// Verify file content
	now := time.Now().Format("2006_01_02")
	path := filepath.Join(caseTmpDir, "personal", "journals", now+".md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read journal file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}

	// Line 1: Parent
	if !strings.HasPrefix(lines[0], "- ") || !strings.Contains(lines[0], "Parent Note") {
		t.Errorf("line 0 incorrect: %q", lines[0])
	}
	// Line 2: Child 1 (no timestamp, NO #inbox)
	if lines[1] != "  - Child 1" {
		t.Errorf("line 1 incorrect (expected no timestamp, no inbox): %q", lines[1])
	}
	// Line 3: Child 2 (with timestamp, NO #inbox)
	matched, _ := regexp.MatchString(`^  - \d{2}:\d{2} Child 2$`, lines[2])
	if !matched {
		t.Errorf("line 2 incorrect (expected timestamp, no inbox): %q", lines[2])
	}
}

func TestStubHandling(t *testing.T) {
	caseTmpDir, err := os.MkdirTemp("", "logseq-stub-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(caseTmpDir)

	mock := &mockTelegramClient{}
	bot := NewBot(mock, filepath.Join(caseTmpDir, ".offset"))
	bot.rootDir = caseTmpDir
	ctx := context.Background()

	// 1. Pre-create file with a single "-" stub
	now := time.Now().Format("2006_01_02")
	path := filepath.Join(caseTmpDir, "personal", "journals", now+".md")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("-"), 0644)

	// 2. Send message
	bot.processMessage(ctx, Update{Message: struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	}{Text: "Clean Note", Chat: struct{ ID int64 `json:"id"` }{ID: 1}}})

	// 3. Verify content
	content, _ := os.ReadFile(path)
	if strings.HasPrefix(string(content), "--") {
		t.Errorf("detected double bullet in content: %q", string(content))
	}
	if !strings.HasPrefix(string(content), "- ") {
		t.Errorf("expected single bullet, got: %q", string(content))
	}
}

func TestToggleAlso(t *testing.T) {
	caseTmpDir, err := os.MkdirTemp("", "logseq-toggle-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(caseTmpDir)

	mock := &mockTelegramClient{}
	bot := NewBot(mock, filepath.Join(caseTmpDir, ".offset"))
	bot.rootDir = caseTmpDir
	ctx := context.Background()

	// 1. Send parent
	bot.processMessage(ctx, Update{Message: struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	}{Text: "Parent", Chat: struct{ ID int64 `json:"id"` }{ID: 1}}})

	// 2. Enable toggle
	bot.processMessage(ctx, Update{Message: struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	}{Text: "toggle also", Chat: struct{ ID int64 `json:"id"` }{ID: 1}}})

	// 3. Send message (should auto-nest, no #inbox)
	bot.processMessage(ctx, Update{Message: struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	}{Text: "Auto Nested", Chat: struct{ ID int64 `json:"id"` }{ID: 1}}})

	// 4. Force timeout
	bot.lastInteractionTime = bot.lastInteractionTime.Add(-6 * time.Minute)

	// 5. Send message (should be top-level, with #inbox)
	bot.processMessage(ctx, Update{Message: struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	}{Text: "Top Level Again", Chat: struct{ ID int64 `json:"id"` }{ID: 1}}})

	// 6. Enable toggle again
	bot.processMessage(ctx, Update{Message: struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	}{Text: "toggle also", Chat: struct{ ID int64 `json:"id"` }{ID: 1}}})

	// 7. Disable toggle immediately
	bot.processMessage(ctx, Update{Message: struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	}{Text: "toggle also", Chat: struct{ ID int64 `json:"id"` }{ID: 1}}})

	// 8. Send message (should be top-level)
	bot.processMessage(ctx, Update{Message: struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	}{Text: "Disabled Manually", Chat: struct{ ID int64 `json:"id"` }{ID: 1}}})

	// Verify file content
	now := time.Now().Format("2006_01_02")
	path := filepath.Join(caseTmpDir, "personal", "journals", now+".md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read journal file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %v", len(lines), lines)
	}

	if !strings.Contains(lines[1], "Auto Nested") || strings.Contains(lines[1], "#inbox") || !strings.HasPrefix(lines[1], "  - ") {
		t.Errorf("line 1 incorrect (should be nested, no inbox): %q", lines[1])
	}
	if !strings.Contains(lines[2], "Top Level Again #inbox") || !strings.HasPrefix(lines[2], "- ") {
		t.Errorf("line 2 incorrect (should be top-level, with inbox): %q", lines[2])
	}
	if !strings.Contains(lines[3], "Disabled Manually #inbox") || !strings.HasPrefix(lines[3], "- ") {
		t.Errorf("line 3 incorrect (should be top-level after manual disable): %q", lines[3])
	}
}
