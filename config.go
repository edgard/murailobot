package main

import (
	"github.com/kelseyhightower/envconfig"
)

// Config holds the configuration variables for the application
type Config struct {
	TelegramToken       string  `envconfig:"telegram_token" required:"true"`     // Token for accessing the Telegram API
	TelegramAdminUID    int64   `envconfig:"telegram_admin_uid" required:"true"` // Telegram Admin User ID
	TelegramUserTimeout float64 `envconfig:"telegram_user_timeout" default:"5"`  // Timeout duration for Telegram users
	OpenAIToken         string  `envconfig:"openai_token" required:"true"`       // Token for accessing the OpenAI API
	OpenAIInstruction   string  `envconfig:"openai_instruction" required:"true"` // Instruction string for OpenAI
	OpenAIModel         string  `envconfig:"openai_model" default:"gpt-4o"`      // Model name for OpenAI
	OpenAITemperature   float32 `envconfig:"openai_temperature" default:"0.5"`   // Temperature setting for OpenAI
	OpenAITopP          float32 `envconfig:"openai_top_p" default:"0.5"`         // TopP setting for OpenAI
	DBName              string  `envconfig:"db_name" default:"storage.db"`       // Database name
}

// NewConfig initializes the configuration by processing environment variables.
func NewConfig() (*Config, error) {
	var config Config

	// Process the environment variables and populate the config struct
	err := envconfig.Process("murailobot", &config)
	if err != nil {
		return nil, WrapError("failed to process environment variables", err)
	}

	return &config, nil
}
