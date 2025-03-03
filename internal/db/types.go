// Package db provides SQLite-based storage for chat history management.
// It implements a simple interface for storing and retrieving chat interactions
// while maintaining proper indexing and data consistency.
package db

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Database configuration defaults define the SQLite operational parameters.
const (
	defaultTempStore   = "MEMORY"         // Temporary storage location (MEMORY or FILE)
	defaultCacheSizeKB = 4000             // SQLite page cache size in kilobytes
	defaultOpTimeout   = 15 * time.Second // Default timeout for database operations
	defaultDSNTimeout  = 5000             // SQLite busy timeout in milliseconds
	defaultMaxOpenConn = 1                // Maximum number of open connections
)

// Database defines the interface for chat history storage operations.
// This interface abstracts the underlying storage implementation,
// allowing for different storage backends if needed.
type Database interface {
	// GetRecent retrieves the most recent chat history entries.
	// The limit parameter controls how many entries to return.
	GetRecent(ctx context.Context, limit int) ([]ChatHistory, error)

	// Save stores a new chat interaction in the database.
	// It records both the user's message and the bot's response.
	Save(ctx context.Context, userID int64, userName string, userMsg, botMsg string) error

	// DeleteAll removes all chat history entries from the database.
	// This is typically used for maintenance or privacy purposes.
	DeleteAll(ctx context.Context) error

	// Close properly closes the database connection.
	// This should be called when shutting down the application.
	Close() error
}

// Config defines database settings that control SQLite behavior.
// These settings affect performance and resource usage.
type Config struct {
	TempStore   string        // Temporary storage location (MEMORY or FILE)
	CacheSizeKB int           // SQLite page cache size in kilobytes
	OpTimeout   time.Duration // Timeout for database operations
}

// SQLiteDB represents a SQLite database connection.
// It implements the Database interface using SQLite as the backend.
type SQLiteDB struct {
	db  *gorm.DB // GORM database connection
	cfg *Config  // Database configuration
}

// ChatHistory represents a single chat interaction record in the database.
// It uses GORM for object-relational mapping and includes proper indexing
// for efficient querying.
type ChatHistory struct {
	gorm.Model           // Includes ID, CreatedAt, UpdatedAt, DeletedAt
	UserID     int64     `gorm:"not null;index"`     // Telegram user ID (indexed)
	UserName   string    `gorm:"type:text"`          // User's display name
	UserMsg    string    `gorm:"not null;type:text"` // User's message
	BotMsg     string    `gorm:"not null;type:text"` // Bot's response
	Timestamp  time.Time `gorm:"not null;index"`     // Message timestamp (indexed)
}
