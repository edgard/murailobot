package interfaces

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot defines the interface for bot operations
type Bot interface {
	// Start begins bot operation
	Start(ctx context.Context) error

	// Stop halts bot operation
	Stop() error

	// GetInfo returns the bot's identification information
	GetInfo() *tgbotapi.User

	// SendMessage sends a message to a chat
	SendMessage(chatID int64, text string) error
}
