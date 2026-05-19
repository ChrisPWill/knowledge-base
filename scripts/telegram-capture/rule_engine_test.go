package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuleEngine(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rule-engine-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	rulesDir := filepath.Join(tmpDir, "personal", "pages")
	os.MkdirAll(rulesDir, 0755)
	rulesFile := filepath.Join(rulesDir, "Telegram Rules.md")

	e := NewRuleEngine(tmpDir)

	// 1. Initial state: File doesn't exist
	if tags := e.ApplyTags("gym"); tags != "gym" {
		t.Errorf("Expected 'gym', got %q", tags)
	}

	// 2. Create rules file
	os.WriteFile(rulesFile, []byte("- #fitness: Gym, Workout\n"), 0644)

	// 3. Apply tags (should load)
	if tags := e.ApplyTags("Time for the Gym"); !strings.Contains(tags, "#fitness") {
		t.Errorf("Expected #fitness tag, got %q", tags)
	}

	// 4. Update rules file
	time.Sleep(10 * time.Millisecond) // Ensure modTime changes
	os.WriteFile(rulesFile, []byte("- #fitness: Gym, Workout\n- #work: Meeting, Call\n"), 0644)

	// 5. Apply tags (should reload)
	if tags := e.ApplyTags("Morning Meeting"); !strings.Contains(tags, "#work") {
		t.Errorf("Expected #work tag, got %q", tags)
	}
}

func TestRuleEngine_EdgeCases(t *testing.T) {
	e := NewRuleEngine(".")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Whole word match only", "Gymnastics", "Gymnastics"},
		{"Case insensitive", "WORKOUT", "WORKOUT"}, // Note: RuleEngine will append if matched, but won't match if it's not whole word
	}

	// Setup rules in memory for test
	e.rules = []Rule{
		{Tag: "#fitness", Keywords: []string{"Gym", "Workout"}},
	}
	e.lastLoaded = time.Now()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := e.ApplyTags(tc.input)
			if tc.name == "Whole word match only" {
				if strings.Contains(result, "#fitness") {
					t.Errorf("Should not have matched %q", tc.input)
				}
			}
			if tc.name == "Case insensitive" {
				if !strings.Contains(result, "#fitness") {
					t.Errorf("Should have matched %q", tc.input)
				}
			}
		})
	}
}
