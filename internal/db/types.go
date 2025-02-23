// Package db provides persistent storage for chat interactions.
package db

import (
	"context"
	"time"
)

type ChatHistory struct {
	ID        int64     `db:"id"`
	UserID    int64     `db:"user_id"`
	UserName  string    `db:"user_name"`
	UserMsg   string    `db:"user_msg"`
	BotMsg    string    `db:"bot_msg"`
	Timestamp time.Time `db:"timestamp"`
}

// Database provides thread-safe chat history operations.
type Database interface {
	// GetRecent returns newest messages first
	GetRecent(ctx context.Context, limit int) ([]ChatHistory, error)

	// Save stores a new chat interaction
	Save(ctx context.Context, userID int64, userName, userMsg, botMsg string) error

	// DeleteAll removes all chat history (cannot be undone)
	DeleteAll(ctx context.Context) error

	// Close releases database resources
	Close() error
}
