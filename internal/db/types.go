// Package db provides SQLite-based storage for chat history.
package db

import (
	"time"

	"gorm.io/gorm"
)

const (
	defaultTempStore   = "MEMORY"
	defaultCacheSizeKB = 4000
	defaultDSNTimeout  = 10000
	defaultMaxOpenConn = 1
)

// Database defines the interface for chat history operations.
type Database interface {
	// GetRecent retrieves the most recent chat history entries.
	GetRecent(limit int) ([]ChatHistory, error)

	// Save stores a new chat interaction.
	Save(userID int64, userName string, userMsg, botMsg string) error

	// DeleteAll removes all chat history entries.
	DeleteAll() error

	// Close closes the database connection.
	Close() error
}

// SQLiteDB implements the Database interface using SQLite.
type SQLiteDB struct {
	db *gorm.DB
}

// ChatHistory represents a chat interaction record.
type ChatHistory struct {
	gorm.Model
	UserID    int64     `gorm:"not null;index"`
	UserName  string    `gorm:"type:text"`
	UserMsg   string    `gorm:"not null;type:text"`
	BotMsg    string    `gorm:"not null;type:text"`
	Timestamp time.Time `gorm:"not null;index"`
}
