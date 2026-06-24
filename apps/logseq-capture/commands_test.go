package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCommandDispatcher(t *testing.T) {
	svc := NewCaptureService(".")
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
			_, _, handled, err := svc.commands.Dispatch(ctx, svc, "client-1", tc.input, SessionState{}, now)
			if err != nil && tc.expected {
				t.Fatalf("Dispatch failed: %v", err)
			}
			if handled != tc.expected {
				t.Errorf("Expected handled=%v, got %v", tc.expected, handled)
			}
		})
	}
}

func TestCommandDispatcherReview(t *testing.T) {
	svc := NewCaptureService(".")
	ctx := context.Background()
	now := time.Now()

	resp, _, handled, err := svc.commands.Dispatch(ctx, svc, "client-1", "/today", SessionState{}, now)
	if err != nil || !handled {
		t.Fatalf("failed to handle /today: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(resp.Reply, "today") {
		t.Errorf("expected review message, got %q", resp.Reply)
	}
}
