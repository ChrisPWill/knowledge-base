package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Initialize structured logging
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	token := os.Getenv("LOGSEQ_CAPTURE_TELEGRAM_API_KEY")
	if token == "" {
		slog.Error("LOGSEQ_CAPTURE_TELEGRAM_API_KEY is not set")
		os.Exit(1)
	}

	client := NewHTTPTelegramClient(token)
	bot := NewBot(client, ".offset")

	// Create a context that is cancelled on SIGINT or SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bot.Run(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			slog.Info("Bot stopped by signal")
			return
		}
		slog.Error("Bot exited with error", "error", err)
		os.Exit(1)
	}
}
