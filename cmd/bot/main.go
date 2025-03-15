package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"calendar-assistant/pkg/config"
	"calendar-assistant/pkg/openai"
	"calendar-assistant/pkg/telegram"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	// Configure logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Calendar Assistant...")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	log.Println("Configuration loaded successfully")

	// Create OpenAI client
	log.Println("Creating OpenAI client...")
	openaiClient := openai.NewClient(cfg)
	log.Println("OpenAI client created successfully")

	// Create Telegram bot
	log.Println("Creating Telegram bot...")
	bot, err := telegram.NewBot(cfg.TelegramBotToken, openaiClient)
	if err != nil {
		log.Fatalf("Failed to create Telegram bot: %v", err)
	}
	log.Println("Telegram bot created successfully")

	// Delete webhook using the underlying BotAPI instance
	log.Println("Deleting any existing webhook...")
	botAPI, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		log.Fatalf("Failed to create BotAPI: %v", err)
	}

	// Enable debug mode
	botAPI.Debug = true

	// Delete webhook
	_, err = botAPI.Request(tgbotapi.DeleteWebhookConfig{
		DropPendingUpdates: true,
	})
	if err != nil {
		log.Fatalf("Failed to delete webhook: %v", err)
	}
	log.Println("Webhook deleted successfully")

	// Wait a moment for the webhook to be fully deleted
	time.Sleep(1 * time.Second)

	// Start the bot in a goroutine
	go func() {
		log.Println("Starting Telegram bot...")
		if err := bot.Start(); err != nil {
			log.Fatalf("Failed to start Telegram bot: %v", err)
		}
	}()

	log.Println("Bot is now running. Press CTRL-C to exit.")

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
}
