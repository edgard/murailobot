package db

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// New constants for database configuration.
const (
	DefaultTempStore    = "MEMORY"
	DefaultCacheSizeKB  = 4000
	DefaultOpTimeout    = 15 * time.Second
	DSNTimeoutMS        = 5000
	DefaultMaxOpenConns = 1
)

// Config defines database settings.
type Config struct {
	TempStore   string
	CacheSizeKB int
	OpTimeout   time.Duration
}

// ChatHistory stores a chat interaction record.
type ChatHistory struct {
	gorm.Model
	UserID    int64     `gorm:"not null;index"`
	UserName  string    `gorm:"type:text"`
	UserMsg   string    `gorm:"not null;type:text"`
	BotMsg    string    `gorm:"not null;type:text"`
	Timestamp time.Time `gorm:"not null;index"`
}

// SQLiteDB represents a SQLite database connection.
type SQLiteDB struct {
	db  *gorm.DB
	cfg *Config
}

// Database interface for chat history operations.
type Database interface {
	GetRecent(ctx context.Context, limit int) ([]ChatHistory, error)
	Save(ctx context.Context, userID int64, userName string, userMsg, botMsg string) error
	DeleteAll(ctx context.Context) error
	Close() error
}
