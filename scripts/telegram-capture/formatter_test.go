package main

import (
	"strings"
	"testing"
	"time"
)

func TestLogseqFormatter(t *testing.T) {
	f := &LogseqFormatter{}
	now := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	state := SessionState{
		LastEntryTime: now.Add(-2 * time.Hour),
	}

	tests := []struct {
		name     string
		block    LogseqBlock
		expected string
	}{
		{
			"Basic top-level",
			LogseqBlock{Text: "Hello", Timestamp: now},
			"- 12:00 Hello #inbox\n",
		},
		{
			"TODO Priority",
			LogseqBlock{Text: "Fix bug", IsTodo: true, Priority: "[#A] ", Timestamp: now},
			"- TODO [#A] 12:00 Fix bug #inbox\n",
		},
		{
			"Nested recent",
			LogseqBlock{Text: "Child", IndentLevel: 1, Timestamp: now},
			"  - Child\n", // Note: state.LastEntryTime is 2h ago, but if I set it closer...
		},
		{
			"Nested old",
			LogseqBlock{Text: "Child", IndentLevel: 1, Timestamp: now},
			"  - 12:00 Child\n",
		},
		{
			"Scheduled",
			LogseqBlock{Text: "Task", ScheduleMarker: " SCHEDULED: <2026-05-20 Wed>", Timestamp: now},
			"- 12:00 Task SCHEDULED: <2026-05-20 Wed> #inbox\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Adjust state for "Nested recent"
			s := state
			if tc.name == "Nested recent" {
				s.LastEntryTime = now.Add(-10 * time.Minute)
			}
			
			result := f.Format(tc.block, s)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestLogseqFormatter_InboxTag(t *testing.T) {
	f := &LogseqFormatter{}
	now := time.Now()
	state := SessionState{}

	tests := []struct {
		text     string
		indent   int
		expected bool // expect #inbox
	}{
		{"No tag", 0, true},
		{"Has #tag", 0, false},
		{"Nested no tag", 1, false},
	}

	for _, tc := range tests {
		block := LogseqBlock{Text: tc.text, IndentLevel: tc.indent, Timestamp: now}
		result := f.Format(block, state)
		hasInbox := strings.Contains(result, "#inbox")
		if hasInbox != tc.expected {
			t.Errorf("Text %q (indent %d): expected #inbox=%v, got %v", tc.text, tc.indent, tc.expected, hasInbox)
		}
	}
}
