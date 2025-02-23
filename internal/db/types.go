package db

import (
	"context"
	"time"
)

// ChatHistory represents a chat interaction record
type ChatHistory struct {
	ID        int64     `db:"id"`
	UserID    int64     `db:"user_id"`
	UserName  string    `db:"user_name"`
	UserMsg   string    `db:"user_msg"`
	BotMsg    string    `db:"bot_msg"`
	Timestamp time.Time `db:"timestamp"`
}

// Database interface defines the required database operations
type Database interface {
	GetRecentChatHistory(ctx context.Context, limit int) ([]ChatHistory, error)
	SaveChatInteraction(ctx context.Context, userID int64, userName, userMsg, botMsg string) error
	DeleteAllChatHistory(ctx context.Context) error
	Close() error
}
