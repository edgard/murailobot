package interfaces

import (
	"context"
)

// BotInfo holds the bot's identification information
type BotInfo struct {
	UserID      int64
	Username    string
	DisplayName string
}

// Bot defines the interface for bot operations
type Bot interface {
	// Start begins bot operation
	Start(ctx context.Context) error

	// Stop halts bot operation
	Stop() error

	// SetName updates bot name information
	SetName(username, displayName string)

	// GetInfo returns the bot's identification information
	GetInfo() BotInfo

	// SendMessage sends a message to a chat
	SendMessage(chatID int64, text string) error
}
