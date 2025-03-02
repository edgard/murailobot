package telegram

import (
	"errors"
	"time"

	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/openai"
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
	OpenAIError  string
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

// Bot implements a Telegram bot with OpenAI capabilities.
type Bot struct {
	api    *tgbotapi.BotAPI
	db     db.Database
	openAI openai.Service
	cfg    *Config
}

var (
	ErrNilConfig        = errors.New("config is nil")
	ErrNilDatabase      = errors.New("database is nil")
	ErrNilOpenAIService = errors.New("OpenAI service is nil")
	ErrNilMessage       = errors.New("message is nil")
	ErrUnauthorized     = errors.New("unauthorized access")
)
