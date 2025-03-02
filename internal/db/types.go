package db

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Database configuration defaults.
const (
	DefaultTempStore   = "MEMORY"
	DefaultCacheSizeKB = 4000
	DefaultOpTimeout   = 15 * time.Second
	DefaultDSNTimeout  = 5000
	DefaultMaxOpenConn = 1
)

// Database defines the interface for chat history storage operations.
type Database interface {
	GetRecent(ctx context.Context, limit int) ([]ChatHistory, error)
	Save(ctx context.Context, userID int64, userName string, userMsg, botMsg string) error
	DeleteAll(ctx context.Context) error
	Close() error
}

// Config defines database settings.
type Config struct {
	TempStore   string
	CacheSizeKB int
	OpTimeout   time.Duration
}

// SQLiteDB represents a SQLite database connection.
type SQLiteDB struct {
	db  *gorm.DB
	cfg *Config
}

// ChatHistory represents a single chat interaction record.
type ChatHistory struct {
	gorm.Model
	UserID    int64     `gorm:"not null;index"`
	UserName  string    `gorm:"type:text"`
	UserMsg   string    `gorm:"not null;type:text"`
	BotMsg    string    `gorm:"not null;type:text"`
	Timestamp time.Time `gorm:"not null;index"`
}
