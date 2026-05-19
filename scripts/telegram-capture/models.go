package main

import (
	"time"
)

// CaptureItem is a generic, transport-agnostic representation of something to be captured.
type CaptureItem struct {
	ID          int64
	Text        string
	Caption     string
	HasMedia    bool
	MediaID     string
	IsVoice     bool
	HasLocation bool
	Latitude    float64
	Longitude   float64
	VenueTitle  string
	VenueAddr   string
	Timestamp   time.Time
	IsEdit      bool
}

// SessionState tracks the context of the user interaction (nesting, timeouts).
type SessionState struct {
	LastProfile         string
	LastJournalFile     string
	LastEntryTime       time.Time
	IsToggledAlso       bool
	LastInteractionTime time.Time
}

// ParsedResult contains the outcome of parsing a CaptureItem.
type ParsedResult struct {
	Entry   string
	Profile string
}
