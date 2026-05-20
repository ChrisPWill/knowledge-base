package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

type mockClientForCommands struct {
	sent []string
}

func (m *mockClientForCommands) GetUpdates(ctx context.Context, offset int, timeout int) ([]Update, error) {
	return nil, nil
}
func (m *mockClientForCommands) SendMessage(ctx context.Context, chatID int64, text string) error {
	m.sent = append(m.sent, text)
	return nil
}
func (m *mockClientForCommands) GetFile(ctx context.Context, fileID string) (string, error) {
	return "", nil
}
func (m *mockClientForCommands) DownloadFile(ctx context.Context, filePath string, destPath string) error {
	return nil
}

func TestCommandDispatcher(t *testing.T) {
	mock := &mockClientForCommands{}
	bot := &Bot{
		client:   mock,
		commands: NewCommandDispatcher(),
		journal:  NewJournalStore("."),
	}
	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		input    string
		expected bool
	}{
		{"/today", true},
		{"HELP", true},
		{"help priority", true},
		{"toggle also", true},
		{"hello world", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			handled, err := bot.commands.Dispatch(ctx, bot, 123, tc.input, now)
			if err != nil {
				t.Fatalf("Dispatch failed: %v", err)
			}
			if handled != tc.expected {
				t.Errorf("Expected handled=%v, got %v", tc.expected, handled)
			}
		})
	}
}

func TestCommandDispatcher_Review(t *testing.T) {
	mock := &mockClientForCommands{}
	bot := NewBot(mock, ".offset")
	ctx := context.Background()
	now := time.Now()

	// Test review handler sends a message
	handled, err := bot.commands.Dispatch(ctx, bot, 123, "/today", now)
	if err != nil || !handled {
		t.Fatalf("Failed to handle /today")
	}

	if len(mock.sent) != 1 || !strings.Contains(mock.sent[0], "today") {
		t.Errorf("Expected review message, got %v", mock.sent)
	}
}
