package main

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

// Config holds the configuration variables
type Config struct {
	TelegramToken       string  `envconfig:"telegram_token" required:"true"`
	TelegramAdminUID    int64   `envconfig:"telegram_admin_uid" required:"true"`
	TelegramUserTimeout float64 `envconfig:"telegram_user_timeout" default:"5"`
	OpenAIToken         string  `envconfig:"openai_token" required:"true"`
	OpenAIInstruction   string  `envconfig:"openai_instruction" required:"true"`
	DBName              string  `envconfig:"db_name" default:"storage.db"`
}

// NewConfig initializes the configuration by processing environment variables.
func NewConfig() (*Config, error) {
	var config Config
	if err := envconfig.Process("murailobot", &config); err != nil {
		return nil, fmt.Errorf("failed to process environment variables: %w", err)
	}
	return &config, nil
}
