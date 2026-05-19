package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type JournalStore struct {
	rootDir string
}

func NewJournalStore(rootDir string) *JournalStore {
	return &JournalStore{rootDir: rootDir}
}

func (s *JournalStore) GetJournalPath(profile string, date time.Time) string {
	dateStr := date.Format("2006_01_02")
	return filepath.Join(s.rootDir, profile, "journals", dateStr+".md")
}

func (s *JournalStore) Append(profile, entry string, timestamp time.Time) (string, error) {
	journalFile := s.GetJournalPath(profile, timestamp)
	journalDir := filepath.Dir(journalFile)

	if err := os.MkdirAll(journalDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", journalDir, err)
	}

	f, err := os.OpenFile(journalFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s: %w", journalFile, err)
	}
	defer f.Close()

	// Handle Logseq's default "-" stub or ensure newline
	info, err := f.Stat()
	if err == nil && info.Size() > 0 {
		content, err := io.ReadAll(f)
		if err == nil {
			trimmed := strings.TrimSpace(string(content))
			if trimmed == "-" || trimmed == "" {
				f.Truncate(0)
				f.Seek(0, 0)
			} else {
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
		return "", fmt.Errorf("failed to write to file %s: %w", journalFile, err)
	}

	return journalFile, nil
}

func (s *JournalStore) Update(filePath, oldContent, newContent string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read journal file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		// Exact match of the whole line (including newline if it was there in the original metadata)
		if line+"\n" == oldContent {
			if newContent == "" {
				lines = append(lines[:i], lines[i+1:]...)
			} else {
				lines[i] = strings.TrimSuffix(newContent, "\n")
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("could not find original entry in %s", filePath)
	}

	newFileContent := strings.Join(lines, "\n")
	return os.WriteFile(filePath, []byte(newFileContent), 0644)
}

func (s *JournalStore) ReadReview(ctx context.Context, date time.Time) (string, error) {
	var reviewText strings.Builder
	for _, profile := range []string{"personal", "work"} {
		journalFile := s.GetJournalPath(profile, date)

		if content, err := os.ReadFile(journalFile); err == nil {
			if reviewText.Len() > 0 {
				reviewText.WriteString("\n")
			}
			profileLabel := strings.Title(profile)
			reviewText.WriteString(fmt.Sprintf("📖 *%s (%s)*\n", profileLabel, s.getDateLabel(date)))
			reviewText.WriteString(string(content))
		}
	}
	return reviewText.String(), nil
}

func (s *JournalStore) getDateLabel(date time.Time) string {
	now := time.Now()
	if date.Format("2006-01-02") == now.Format("2006-01-02") {
		return "today"
	}
	if date.Format("2006-01-02") == now.AddDate(0, 0, -1).Format("2006-01-02") {
		return "yesterday"
	}
	return date.Format("2006-01-02")
}
