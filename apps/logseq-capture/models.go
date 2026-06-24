package main

import (
	"time"
)

type RequestKind string

const (
	RequestKindCapture RequestKind = "capture"
	RequestKindCommand RequestKind = "command"
	RequestKindReview  RequestKind = "review"
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

type CaptureRequest struct {
	ClientID  string       `json:"client_id"`
	Kind      RequestKind  `json:"kind,omitempty"`
	Text      string       `json:"text,omitempty"`
	ReviewDay string       `json:"review_day,omitempty"`
	Timestamp *time.Time   `json:"-"`
	Item      *CaptureItem `json:"-"`
}

type CaptureResponse struct {
	OK          bool   `json:"ok"`
	Reply       string `json:"reply"`
	Error       string `json:"error,omitempty"`
	Profile     string `json:"profile,omitempty"`
	Entry       string `json:"entry,omitempty"`
	JournalPath string `json:"journal_path,omitempty"`
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
