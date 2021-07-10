package main

import (
	"time"

	"gorm.io/gorm"
)

type MessageRef struct {
	gorm.Model
	MessageID int
	ChatID    int64
	LastUsed  time.Time
}

type User struct {
	gorm.Model
	UserID   int
	LastUsed time.Time
}

type Config struct {
	AuthToken     string  `mapstructure:"auth_token"`
	UpdateTimeout int     `mapstructure:"update_timeout"`
	UserTimeout   float64 `mapstructure:"user_timeout"`
	TelegramDebug bool    `mapstructure:"telegram_debug"`
	DBName        string  `mapstructure:"db_name"`
}
