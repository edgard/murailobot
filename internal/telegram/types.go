package telegram

import (
	"context"
	"time"

	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/db"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Magic number constants.
const (
	DefaultTypingInterval = 5 * time.Second
	DefaultUpdateOffset   = 0
	DefaultUpdateTimeout  = 30
)

// Messages stores bot response templates.
type Messages struct {
	Welcome      string `yaml:"welcome"`
	Unauthorized string `yaml:"unauthorized"`
	GeneralError string `yaml:"general_error"`
	AIError      string `yaml:"ai_error"`
	HistoryReset string `yaml:"history_reset"`
	Provide      string `yaml:"provide_message"`
	Timeout      string `yaml:"timeout"`
}

// Config holds bot settings.
type Config struct {
	Token    string   `yaml:"token"`
	AdminID  int64    `yaml:"admin_id"`
	Messages Messages `yaml:"messages"`
}

// Bot represents a telegram bot.
type Bot struct {
	api *tgbotapi.BotAPI
	db  db.Database
	ai  ai.Service
	cfg *Config
}

// Service defines telegram bot operations.
type Service interface {
	Start(ctx context.Context) error
	Stop() error
	SendContinuousTyping(ctx context.Context, chatID int64)
}
