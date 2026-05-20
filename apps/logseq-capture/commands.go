package main

import (
	"context"
	"strings"
	"time"
)

type CommandHandler func(ctx context.Context, b *Bot, chatID int64, text string, now time.Time) error

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

func (d *CommandDispatcher) Dispatch(ctx context.Context, b *Bot, chatID int64, text string, now time.Time) (bool, error) {
	lowerMsg := strings.ToLower(strings.TrimSpace(text))
	
	// Exact matches
	if handler, ok := d.handlers[lowerMsg]; ok {
		return true, handler(ctx, b, chatID, lowerMsg, now)
	}

	// Prefix matches (e.g. help priority)
	for cmd, handler := range d.handlers {
		if strings.HasPrefix(lowerMsg, cmd+" ") {
			return true, handler(ctx, b, chatID, lowerMsg, now)
		}
	}

	return false, nil
}

func handleHelpCommand(ctx context.Context, b *Bot, chatID int64, text string, now time.Time) error {
	return b.sendHelp(ctx, chatID, text)
}

func handleReviewCommand(ctx context.Context, b *Bot, chatID int64, text string, now time.Time) error {
	return b.sendReview(ctx, chatID, text, now)
}

func handleToggleCommand(ctx context.Context, b *Bot, chatID int64, text string, now time.Time) error {
	return b.toggleAlso(ctx, chatID, now)
}
