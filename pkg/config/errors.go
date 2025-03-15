package config

import "errors"

// Error definitions
var (
	ErrMissingTelegramToken = errors.New("missing Telegram bot token")
	ErrMissingOpenAIKey     = errors.New("missing OpenAI API key")
)
