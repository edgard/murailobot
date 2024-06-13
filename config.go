package main

import (
	"github.com/kelseyhightower/envconfig"
)

// Config holds the configuration variables
type Config struct {
	TelegramToken string  `envconfig:"telegram_token" required:"true"`
	OpenAIToken   string  `envconfig:"openai_token" required:"true"`
	UserTimeout   float64 `envconfig:"user_timeout" default:"5"`
	DBName        string  `envconfig:"db_name" default:"storage.db"`
	AdminUID      int64   `envconfig:"admin_uid" required:"true"`
}

var config Config

func loadConfig() error {
	if err := envconfig.Process("murailobot", &config); err != nil {
		return err
	}
	return nil
}
