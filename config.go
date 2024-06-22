package main

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

// Config holds the configuration variables for the application
type Config struct {
	TelegramToken       string  `envconfig:"telegram_token" required:"true"`     // Token for accessing the Telegram API
	TelegramAdminUID    int64   `envconfig:"telegram_admin_uid" required:"true"` // Telegram Admin User ID
	TelegramUserTimeout float64 `envconfig:"telegram_user_timeout" default:"5"`  // Timeout duration for Telegram users
	OpenAIToken         string  `envconfig:"openai_token" required:"true"`       // Token for accessing the OpenAI API
	OpenAIInstruction   string  `envconfig:"openai_instruction" required:"true"` // Instruction string for OpenAI
	DBName              string  `envconfig:"db_name" default:"storage.db"`       // Database name
}

// NewConfig initializes the configuration by processing environment variables.
func NewConfig() (*Config, error) {
	var config Config

	// Process the environment variables and populate the config struct
	if err := envconfig.Process("murailobot", &config); err != nil {
		return nil, WrapError(fmt.Errorf("failed to process environment variables: %w", err))
	}

	return &config, nil
}
