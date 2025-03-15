package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	TelegramBotToken  string
	OpenAIAPIKey      string
	OpenAIAssistantID string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	telegramBotToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if telegramBotToken == "" {
		return nil, ErrMissingTelegramToken
	}

	openAIAPIKey := os.Getenv("OPENAI_API_KEY")
	if openAIAPIKey == "" {
		return nil, ErrMissingOpenAIKey
	}

	// Assistant ID is optional
	openAIAssistantID := os.Getenv("OPENAI_ASSISTANT_ID")

	return &Config{
		TelegramBotToken:  telegramBotToken,
		OpenAIAPIKey:      openAIAPIKey,
		OpenAIAssistantID: openAIAssistantID,
	}, nil
}
