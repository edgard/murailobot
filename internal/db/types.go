// Package db provides SQLite-based storage for chat history management.
package db

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Database configuration constants define the SQLite operational parameters.
const (
	defaultTempStore   = "MEMORY"         // Temporary storage location (MEMORY or FILE)
	defaultCacheSizeKB = 4000             // SQLite page cache size in kilobytes
	defaultOpTimeout   = 10 * time.Second // Default timeout for database operations
	defaultDSNTimeout  = 10000            // SQLite busy timeout in milliseconds
	defaultMaxOpenConn = 1                // Maximum number of open connections
)

// Database defines the interface for chat history storage operations.
type Database interface {
	// GetRecent retrieves the most recent chat history entries.
	GetRecent(ctx context.Context, limit int) ([]ChatHistory, error)

	// Save stores a new chat interaction in the database.
	Save(ctx context.Context, userID int64, userName string, userMsg, botMsg string) error

	// DeleteAll removes all chat history entries from the database.
	DeleteAll(ctx context.Context) error

	// Close properly closes the database connection.
	Close() error
}

// Config defines database settings that control SQLite behavior.
type Config struct {
	TempStore   string        // Temporary storage location (MEMORY or FILE)
	CacheSizeKB int           // SQLite page cache size in kilobytes
	OpTimeout   time.Duration // Timeout for database operations
}

// SQLiteDB represents a SQLite database connection that implements the Database interface.
type SQLiteDB struct {
	db  *gorm.DB // GORM database connection
	cfg *Config  // Database configuration
}

// ChatHistory represents a single chat interaction record in the database.
type ChatHistory struct {
	gorm.Model           // Includes ID, CreatedAt, UpdatedAt, DeletedAt
	UserID     int64     `gorm:"not null;index"`     // Telegram user ID (indexed)
	UserName   string    `gorm:"type:text"`          // User's display name
	UserMsg    string    `gorm:"not null;type:text"` // User's message
	BotMsg     string    `gorm:"not null;type:text"` // Bot's response
	Timestamp  time.Time `gorm:"not null;index"`     // Message timestamp (indexed)
}
