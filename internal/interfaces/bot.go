package interfaces

import (
	"context"
)

// Bot defines the interface for bot operations
type Bot interface {
	// Start begins bot operation
	Start(ctx context.Context) error

	// Stop halts bot operation
	Stop() error

	// GetID returns the bot's unique identifier
	GetID() int64

	// GetUserName returns the bot's username
	GetUserName() string

	// GetFirstName returns the bot's first name
	GetFirstName() string

	// SendMessage sends a message to a chat
	SendMessage(chatID int64, text string) error
}
