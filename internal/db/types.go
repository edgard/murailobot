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
	hoursInDay         = 24 // Hours in a day for date range calculations
)

// Database defines the interface for chat history operations.
type Database interface {
	// GetRecent retrieves the most recent chat history entries.
	GetRecent(limit int) ([]ChatHistory, error)

	// Save stores a new chat interaction.
	Save(userID int64, userMsg, botMsg string) error

	// SaveGroupMessage stores a message from a group chat.
	SaveGroupMessage(groupID int64, groupName string, userID int64, message string) error

	// GetGroupMessagesInTimeRange retrieves all group messages within a time range.
	GetGroupMessagesInTimeRange(start, end time.Time) ([]GroupMessage, error)

	// SaveUserAnalysis stores personality/behavioral analysis for a user.
	SaveUserAnalysis(analysis *UserAnalysis) error

	// GetUserAnalysesInTimeRange retrieves user analyses within a time range.
	GetUserAnalysesInTimeRange(start, end time.Time) ([]UserAnalysis, error)

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
	UserMsg   string    `gorm:"not null;type:text"`
	BotMsg    string    `gorm:"not null;type:text"`
	Timestamp time.Time `gorm:"not null;index"`
}

// GroupMessage represents a message from a group chat.
type GroupMessage struct {
	gorm.Model
	GroupID   int64     `gorm:"not null;index"`
	GroupName string    `gorm:"type:text"`
	UserID    int64     `gorm:"not null;index"`
	Message   string    `gorm:"not null;type:text"`
	Timestamp time.Time `gorm:"not null;index"`
}

// UserAnalysis represents a behavioral/personality analysis for a user.
type UserAnalysis struct {
	gorm.Model
	UserID             int64     `gorm:"not null;index"`
	Date               time.Time `gorm:"not null;index"`
	CommunicationStyle string    `gorm:"type:text"`
	PersonalityTraits  string    `gorm:"type:text"`
	BehavioralPatterns string    `gorm:"type:text"`
	WordChoicePatterns string    `gorm:"type:text"`
	InteractionHabits  string    `gorm:"type:text"`
	UniqueQuirks       string    `gorm:"type:text"`
	EmotionalTriggers  string    `gorm:"type:text"`
	MessageCount       int       `gorm:"not null"`
}
