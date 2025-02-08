package main

import (
	"github.com/kelseyhightower/envconfig"
)

// Config holds the configuration variables for the application.
// All environment variables use the prefix "murailobot".
type Config struct {
	TelegramToken       string  `envconfig:"telegram_token" required:"true"`     // Telegram API token.
	TelegramAdminUID    int64   `envconfig:"telegram_admin_uid" required:"true"` // Admin user ID.
	TelegramUserTimeout float64 `envconfig:"telegram_user_timeout" default:"5"`  // Timeout (in minutes) for Telegram users.
	OpenAIToken         string  `envconfig:"openai_token" required:"true"`       // OpenAI API token.
	OpenAIInstruction   string  `envconfig:"openai_instruction" required:"true"` // Instruction for OpenAI.
	OpenAIModel         string  `envconfig:"openai_model" default:"gpt-4o"`      // OpenAI model name.
	OpenAITemperature   float32 `envconfig:"openai_temperature" default:"0.5"`   // OpenAI temperature.
	OpenAITopP          float32 `envconfig:"openai_top_p" default:"0.5"`         // OpenAI TopP.
	DBName              string  `envconfig:"db_name" default:"storage.db"`       // SQLite database name.
}

// NewConfig initializes configuration by reading environment variables.
func NewConfig() (*Config, error) {
	var config Config
	if err := envconfig.Process("murailobot", &config); err != nil {
		return nil, WrapError("failed to process environment variables", err)
	}
	if config.TelegramUserTimeout <= 0 {
		return nil, WrapError("telegram_user_timeout must be greater than 0")
	}
	return &config, nil
}
