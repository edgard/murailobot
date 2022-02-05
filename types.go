package main

import (
	"time"
)

var db_schema = `
CREATE TABLE IF NOT EXISTS message_ref (
    id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    message_id INTEGER NOT NULL,
    chat_id INTEGER NOT NULL,
    last_used DATETIME
);

CREATE TABLE IF NOT EXISTS user (
    user_id INTEGER NOT NULL PRIMARY KEY,
    last_used DATETIME
)`

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
}
