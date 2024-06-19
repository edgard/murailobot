package main

import (
	"github.com/kelseyhightower/envconfig"
)

// Config holds the configuration variables
type Config struct {
	TelegramToken       string  `envconfig:"telegram_token" required:"true"`
	TelegramAdminUID    int64   `envconfig:"telegram_admin_uid" required:"true"`
	TelegramUserTimeout float64 `envconfig:"telegram_user_timeout" default:"5"`
	OpenAIToken         string  `envconfig:"openai_token" required:"true"`
	OpenAIInstruction   string  `envconfig:"openai_instruction" required:"true"`
}

// NewConfig initializes the configuration by processing environment variables.
func NewConfig() (*Config, error) {
	var config Config
	if err := envconfig.Process("murailobot", &config); err != nil {
		return nil, err
	}
	return &config, nil
}
