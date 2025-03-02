package telegram

import (
	"errors"
	"time"

	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/db"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Default telegram bot configuration values.
const (
	DefaultUpdateOffset   = 0
	DefaultUpdateTimeout  = 60
	DefaultTypingInterval = 5 * time.Second
)

// Messages defines configurable bot response templates.
type Messages struct {
	Welcome      string
	Unauthorized string
	Provide      string
	AIError      string
	GeneralError string
	HistoryReset string
	Timeout      string
}

// Config holds bot configuration.
type Config struct {
	Token    string
	AdminID  int64
	Messages Messages
}

// Bot implements a Telegram bot with AI capabilities.
type Bot struct {
	api *tgbotapi.BotAPI
	db  db.Database
	ai  ai.Service
	cfg *Config
}

var (
	ErrNilConfig    = errors.New("config is nil")
	ErrNilDatabase  = errors.New("database is nil")
	ErrNilAIService = errors.New("AI service is nil")
	ErrNilMessage   = errors.New("message is nil")
	ErrUnauthorized = errors.New("unauthorized access")
)
