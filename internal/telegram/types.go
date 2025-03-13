package telegram

import (
	"time"

	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/scheduler"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	defaultUpdateOffset   = 0
	defaultUpdateTimeout  = 60
	defaultTypingInterval = 5 * time.Second
	defaultRetryAttempts  = 3
	defaultRetryDelay     = 500 * time.Millisecond
	hoursInDay            = 24
)

// messages defines bot response templates.
type messages struct {
	Welcome      string
	Unauthorized string
	Provide      string
	GeneralError string
	HistoryReset string
}

// botConfig holds bot authentication and response configuration.
type botConfig struct {
	Token    string
	AdminID  int64
	Messages messages
}

// Bot implements a Telegram bot with AI capabilities.
type Bot struct {
	api       *tgbotapi.BotAPI
	db        db.Database
	ai        ai.Service
	scheduler scheduler.Scheduler
	cfg       *botConfig
	running   chan struct{}
}
