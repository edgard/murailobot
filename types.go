package main

import (
	"time"
)

type MessageRef struct {
	ID        int       `db:"id"`
	MessageID int       `db:"message_id"`
	ChatID    int64     `db:"chat_id"`
	LastUsed  time.Time `db:"last_used"`
}

type User struct {
	UserID   int       `db:"user_id"`
	LastUsed time.Time `db:"last_used"`
}

type Config struct {
	AuthToken     string  `envconfig:"auth_token" required:"true"`
	UpdateTimeout int     `envconfig:"update_timeout" default:"60"`
	UserTimeout   float64 `envconfig:"user_timeout" default:"5"`
	TelegramDebug bool    `envconfig:"telegram_debug" default:"false"`
	DBName        string  `envconfig:"db_name" default:"storage.db"`
	OpenAIToken   string  `envconfig:"openai_token" required:"true"`
}
