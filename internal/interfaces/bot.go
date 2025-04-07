package interfaces

import (
	"context"

	"github.com/edgard/murailobot/internal/models"
)

// Bot defines the interface for bot operations
type Bot interface {
	// Configure sets up basic bot configuration
	Configure(token string, adminID int64, maxContextSize int) error

	// SetServices configures bot with required service dependencies
	SetServices(ai AI, db DB, scheduler Scheduler) error

	// SetCommands sets available bot commands
	SetCommands(commands map[string]string) error

	// SetTemplates sets message templates
	SetTemplates(templates map[string]string) error

	// Start begins bot operation
	Start(ctx context.Context) error

	// Stop halts bot operation
	Stop() error

	// GetBotInfo returns bot identification information
	GetBotInfo() models.BotInfo

	// SendMessage sends a message to a chat
	SendMessage(ctx context.Context, chatID int64, text string) error
}
