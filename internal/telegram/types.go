// Package telegram implements a Telegram bot service that integrates with
// AI capabilities for chat interactions. It handles message processing,
// user management, and bot commands.
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

// BotService defines the interface for Telegram bot operations.
// It provides methods for starting and stopping the bot, handling
// incoming messages, and sending responses to users.
type BotService interface {
	// Start begins the bot's operation, listening for updates
	// and processing messages. It runs until the context is cancelled
	// or Stop is called.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the bot, ensuring all pending
	// operations are completed.
	Stop() error

	// HandleMessage processes an incoming Telegram update,
	// which may contain a message, command, or other content.
	HandleMessage(update *gotgbot.Update) error

	// SendMessage sends a text message to the specified chat.
	// It handles message formatting and length validation.
	SendMessage(chatID int64, text string) error

	// SendTypingAction shows the "typing" status in the specified chat,
	// indicating the bot is preparing a response.
	SendTypingAction(chatID int64) error
}

// bot implements the BotService interface, providing a concrete
// implementation of a Telegram bot with AI capabilities.
type bot struct {
	*gotgbot.Bot                       // Embedded Telegram bot client
	updater      *ext.Updater          // Handles incoming updates from Telegram
	db           db.Database           // Persistent storage for chat history
	ai           ai.Service            // AI service for generating responses
	cfg          *config.Config        // Bot configuration
	breaker      *utils.CircuitBreaker // Circuit breaker for API calls
}
