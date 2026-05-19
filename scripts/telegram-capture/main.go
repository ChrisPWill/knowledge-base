package main

import (
	"fmt"
	"os"
)

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		fmt.Println("Error: TELEGRAM_BOT_TOKEN is not set.")
		os.Exit(1)
	}

	client := NewHTTPTelegramClient(token)
	bot := NewBot(client, ".offset")

	if err := bot.Run(); err != nil {
		fmt.Printf("Bot exited with error: %v\n", err)
		os.Exit(1)
	}
}
