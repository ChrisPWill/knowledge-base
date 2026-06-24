package main

import (
	"context"
	"strings"
	"time"
)

type CommandHandler func(ctx context.Context, svc *CaptureService, clientID string, text string, state SessionState, now time.Time) (CaptureResponse, SessionState, error)

type CommandDispatcher struct {
	handlers map[string]CommandHandler
}

func NewCommandDispatcher() *CommandDispatcher {
	d := &CommandDispatcher{
		handlers: make(map[string]CommandHandler),
	}
	d.registerDefaultHandlers()
	return d
}

func (d *CommandDispatcher) registerDefaultHandlers() {
	d.handlers["help"] = handleHelpCommand
	d.handlers["/today"] = handleReviewCommand
	d.handlers["/yesterday"] = handleReviewCommand
	d.handlers["toggle also"] = handleToggleCommand
}

func (d *CommandDispatcher) Dispatch(ctx context.Context, svc *CaptureService, clientID string, text string, state SessionState, now time.Time) (CaptureResponse, SessionState, bool, error) {
	lowerMsg := strings.ToLower(strings.TrimSpace(text))

	// Exact matches
	if handler, ok := d.handlers[lowerMsg]; ok {
		resp, nextState, err := handler(ctx, svc, clientID, lowerMsg, state, now)
		return resp, nextState, true, err
	}

	// Prefix matches (e.g. help priority)
	for cmd, handler := range d.handlers {
		if strings.HasPrefix(lowerMsg, cmd+" ") {
			resp, nextState, err := handler(ctx, svc, clientID, lowerMsg, state, now)
			return resp, nextState, true, err
		}
	}

	return CaptureResponse{}, state, false, nil
}

func handleHelpCommand(ctx context.Context, svc *CaptureService, clientID string, text string, state SessionState, now time.Time) (CaptureResponse, SessionState, error) {
	return svc.helpResponse(text), state, nil
}

func handleReviewCommand(ctx context.Context, svc *CaptureService, clientID string, text string, state SessionState, now time.Time) (CaptureResponse, SessionState, error) {
	day := "today"
	if text == "/yesterday" {
		day = "yesterday"
	}
	resp, err := svc.handleReview(day, now)
	return resp, state, err
}

func handleToggleCommand(ctx context.Context, svc *CaptureService, clientID string, text string, state SessionState, now time.Time) (CaptureResponse, SessionState, error) {
	resp, nextState := svc.toggleAlsoResponse(state, now)
	return resp, nextState, nil
}
