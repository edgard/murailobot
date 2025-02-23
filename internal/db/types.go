// Package db provides database operations for storing and retrieving chat interactions.
// It defines the core data structures and interfaces for persistent storage.
package db

import (
	"context"
	"time"
)

// ChatHistory represents a single chat interaction record between a user and the bot.
// Each record contains the complete context of the interaction including user details,
// the original message, bot's response, and timing information.
type ChatHistory struct {
	ID        int64     `db:"id"`        // Unique identifier for the chat interaction
	UserID    int64     `db:"user_id"`   // Telegram user ID
	UserName  string    `db:"user_name"` // User's current Telegram username
	UserMsg   string    `db:"user_msg"`  // Original message from the user
	BotMsg    string    `db:"bot_msg"`   // Bot's response to the user
	Timestamp time.Time `db:"timestamp"` // When the interaction occurred
}

// Database defines the interface for chat history persistence operations.
// Implementations must be safe for concurrent use by multiple goroutines.
type Database interface {
	// GetRecentChatHistory retrieves the most recent chat interactions.
	// The limit parameter controls how many records to return.
	// Results are ordered by timestamp descending (newest first).
	GetRecentChatHistory(ctx context.Context, limit int) ([]ChatHistory, error)

	// SaveChatInteraction stores a new chat interaction in the database.
	// It automatically sets the timestamp to the current time.
	SaveChatInteraction(ctx context.Context, userID int64, userName, userMsg, botMsg string) error

	// DeleteAllChatHistory removes all chat history records from the database.
	// This operation cannot be undone.
	DeleteAllChatHistory(ctx context.Context) error

	// Close releases any resources held by the database connection.
	// After closing, no other methods should be called.
	Close() error
}
