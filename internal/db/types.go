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
	Save(userID int64, userName string, userMsg, botMsg string) error

	// SaveGroupMessage stores a message from a group chat.
	SaveGroupMessage(groupID int64, groupName string, userID int64, userName string, message string) error

	// GetMessagesByUserInTimeRange retrieves all messages from a user within a time range.
	GetMessagesByUserInTimeRange(userID int64, start, end time.Time) ([]GroupMessage, error)

	// GetActiveUsersInTimeRange retrieves all users who sent messages in the time range.
	GetActiveUsersInTimeRange(start, end time.Time) (map[int64]string, error)

	// SaveUserAnalysis stores personality/behavioral analysis for a user.
	SaveUserAnalysis(analysis *UserAnalysis) error

	// GetUserAnalysesByDateRange retrieves user analyses within a date range.
	GetUserAnalysesByDateRange(start, end time.Time) ([]UserAnalysis, error)

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

// GroupMessage represents a message from a group chat.
type GroupMessage struct {
	gorm.Model
	GroupID   int64     `gorm:"not null;index"`
	GroupName string    `gorm:"type:text"`
	UserID    int64     `gorm:"not null;index"`
	UserName  string    `gorm:"type:text"`
	Message   string    `gorm:"not null;type:text"`
	Timestamp time.Time `gorm:"not null;index"`
}

// UserAnalysis represents a daily behavioral/personality analysis for a user.
type UserAnalysis struct {
	gorm.Model
	UserID             int64     `gorm:"not null;index"`
	UserName           string    `gorm:"type:text"`
	Date               time.Time `gorm:"not null;index"`
	CommunicationStyle string    `gorm:"type:text"` // Formal, casual, direct, etc.
	PersonalityTraits  string    `gorm:"type:text"` // JSON array of identified traits
	BehavioralPatterns string    `gorm:"type:text"` // JSON array of observed patterns
	WordChoices        string    `gorm:"type:text"` // JSON object of notable word usage patterns
	InteractionHabits  string    `gorm:"type:text"` // JSON object of interaction preferences
	Quirks             string    `gorm:"type:text"` // JSON array of unique characteristics
	Mood               string    `gorm:"type:text"` // JSON object with overall mood and variations
	MessageCount       int       `gorm:"not null"`  // Number of messages analyzed
}
