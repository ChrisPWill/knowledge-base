package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

type runtimeDeps struct {
	newTelegramClient func(token string) TelegramClient
	newBot            func(client TelegramClient, offsetFile string) *Bot
	newService        func(rootDir string) *CaptureService
	serveHTTP         func(ctx context.Context, service *CaptureService, addr string) error
	newDaemonClient   func(addr string) *daemonClient
	stdin             *os.File
	stdout            *os.File
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps := runtimeDeps{
		newTelegramClient: func(token string) TelegramClient { return NewHTTPTelegramClient(token) },
		newBot:            NewBot,
		newService:        NewCaptureService,
		serveHTTP: func(ctx context.Context, service *CaptureService, addr string) error {
			return newAPIServer(service).serve(ctx, addr)
		},
		newDaemonClient: newDaemonClient,
		stdin:           os.Stdin,
		stdout:          os.Stdout,
	}

	if err := run(ctx, os.Args[1:], deps); err != nil {
		if errors.Is(err, context.Canceled) {
			slog.Info("Capture process stopped by signal")
			return
		}
		slog.Error("Capture process exited with error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, deps runtimeDeps) error {
	if len(args) == 0 {
		return runTelegram(ctx, deps)
	}

	switch args[0] {
	case "serve":
		service := deps.newService(".")
		addr := daemonAddr()
		slog.Info("Starting local capture daemon", "address", addr)
		return deps.serveHTTP(ctx, service, addr)
	case "send":
		if len(args) < 2 {
			return fmt.Errorf("usage: logseq-capture send <text>")
		}
		resp, err := deps.newDaemonClient(daemonAddr()).capture(ctx, "cli", strings.Join(args[1:], " "))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(deps.stdout, resp.Reply)
		return err
	case "command":
		if len(args) < 2 {
			return fmt.Errorf("usage: logseq-capture command <text>")
		}
		resp, err := deps.newDaemonClient(daemonAddr()).command(ctx, "cli", strings.Join(args[1:], " "))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(deps.stdout, resp.Reply)
		return err
	case "review":
		if len(args) != 2 {
			return fmt.Errorf("usage: logseq-capture review <today|yesterday>")
		}
		resp, err := deps.newDaemonClient(daemonAddr()).review(ctx, "cli", args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(deps.stdout, resp.Reply)
		return err
	case "status":
		status, err := deps.newDaemonClient(daemonAddr()).status(ctx)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(deps.stdout, status)
		return err
	case "cli":
		return runInteractiveCLI(ctx, deps.newDaemonClient(daemonAddr()), "cli-repl", deps.stdin, deps.stdout)
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func runTelegram(ctx context.Context, deps runtimeDeps) error {
	token := os.Getenv("LOGSEQ_CAPTURE_TELEGRAM_API_KEY")
	if token == "" {
		return fmt.Errorf("LOGSEQ_CAPTURE_TELEGRAM_API_KEY is not set")
	}

	client := deps.newTelegramClient(token)
	bot := deps.newBot(client, ".offset")
	slog.Info("Starting Telegram capture runtime")
	return bot.Run(ctx)
}
