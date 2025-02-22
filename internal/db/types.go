package db

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/edgard/murailobot/internal/resilience"
	"github.com/jmoiron/sqlx"
)

var (
	// ErrDatabase represents database-related errors
	ErrDatabase = errors.New("database error")

	// Default timeouts
	defaultOperationTimeout     = 5 * time.Second
	defaultLongOperationTimeout = 30 * time.Second

	// SQL statements for schema setup
	pragmas = []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA cache_size=-2000", // Use up to 2MB of memory for cache
	}

	createChatHistoryTimestampIndex = `
		CREATE INDEX IF NOT EXISTS idx_chat_history_timestamp ON chat_history(timestamp)`

	createChatHistoryUserIDIndex = `
		CREATE INDEX IF NOT EXISTS idx_chat_history_user_id ON chat_history(user_id)`
)

// Config holds database configuration
type Config struct {
	Name            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	MaxMessageSize  int
}

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

// DB implements the Database interface
type DB struct {
	*sqlx.DB
	config  *Config
	breaker *resilience.CircuitBreaker
}

// getChatHistoryTableSchema returns the chat history table schema with configurable message size
func getChatHistoryTableSchema(maxMessageSize int) string {
	return `
		CREATE TABLE IF NOT EXISTS chat_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			user_name TEXT NOT NULL,
			user_msg TEXT NOT NULL CHECK(length(user_msg) <= ` + strconv.Itoa(maxMessageSize) + `),
			bot_msg TEXT NOT NULL CHECK(length(bot_msg) <= ` + strconv.Itoa(maxMessageSize) + `),
			timestamp DATETIME NOT NULL
		)`
}
