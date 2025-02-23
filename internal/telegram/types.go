package telegram

import (
	"context"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/utils"
)

// BotService defines the interface for Telegram bot operations
type BotService interface {
	Start(ctx context.Context) error
	Stop() error
	HandleMessage(update *gotgbot.Update) error
	SendMessage(chatID int64, text string) error
	SendTypingAction(chatID int64) error
}

// bot represents a Telegram bot instance
type bot struct {
	*gotgbot.Bot
	updater *ext.Updater
	db      db.Database
	ai      ai.Service
	cfg     *config.Config
	breaker *utils.CircuitBreaker
}
