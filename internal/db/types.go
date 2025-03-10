// Package db provides storage functionality for chat conversations and analytics.
// It currently implements a SQLite-based storage backend with support for:
// - Chat history management
// - Group message tracking
// - User behavior analysis
// - Context-aware operations
package db

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Database configuration constants.
const (
	defaultMaxOpenConn = 1 // Maximum number of open connections
)

var (
	// ErrInvalidLimit is returned when the limit parameter is invalid.
	ErrInvalidLimit = errors.New("invalid limit")
	// ErrZeroTimeValue is returned when a time value is zero.
	ErrZeroTimeValue = errors.New("zero time values not allowed")
	// ErrInvalidTimeRange is returned when start time is after end time.
	ErrInvalidTimeRange = errors.New("start time must be before end time")
	// ErrTimeRangeExceeded is returned when time range exceeds maximum allowed duration.
	ErrTimeRangeExceeded = errors.New("time range exceeds maximum allowed duration")
	// ErrDatabaseOperation is returned when a database operation fails.
	ErrDatabaseOperation = errors.New("database operation failed")
)

// Database defines the interface for chat history and analytics operations.
type Database interface {
	// GetRecent retrieves the most recent chat history entries.
	// It returns up to 'limit' entries ordered by timestamp descending.
	GetRecent(limit int) ([]ChatHistory, error)

	// Save stores a new chat interaction between a user and the bot.
	// It returns an error if the storage operation fails.
	Save(userID int64, userMsg, botMsg string) error

	// SaveGroupMessage stores a message from a group chat.
	// The groupName is stored to track group name changes over time.
	// It returns an error if the storage operation fails.
	SaveGroupMessage(groupID int64, groupName string, userID int64, message string) error

	// GetGroupMessagesInTimeRange retrieves group messages between start and end times.
	// Times should be in UTC to ensure consistent queries across timezones.
	GetGroupMessagesInTimeRange(start, end time.Time) ([]GroupMessage, error)

	// SaveUserAnalysis stores personality/behavioral analysis for a user.
	// Analysis timestamps are stored in UTC.
	// It returns an error if the storage operation fails.
	SaveUserAnalysis(analysis *UserAnalysis) error

	// GetUserAnalysesInTimeRange retrieves user analyses between start and end times.
	// Times should be in UTC to ensure consistent queries across timezones.
	GetUserAnalysesInTimeRange(start, end time.Time) ([]UserAnalysis, error)

	// DeleteAll removes all stored data, including chat history, group messages, and analyses.
	// This operation cannot be undone.
	// It returns an error if the deletion fails.
	DeleteAll() error

	// Close releases database resources and closes the connection.
	// The database instance should not be used after calling Close.
	Close() error
}

// sqliteDB implements the Database interface using SQLite.
type sqliteDB struct {
	db *gorm.DB
}

// ChatHistory represents a single interaction between a user and the bot.
// It stores both the user's message and the bot's response along with metadata.
type ChatHistory struct {
	gorm.Model
	// Core fields
	UserID    int64     `gorm:"not null;index" json:"user_id"`   // User identifier from Telegram
	Timestamp time.Time `gorm:"not null;index" json:"timestamp"` // When the interaction occurred

	// Message content
	UserMsg string `gorm:"not null;type:text" json:"user_message"` // Original message from user
	BotMsg  string `gorm:"not null;type:text" json:"bot_message"`  // Bot's response to user
}

// GroupMessage represents a message sent in a group chat.
// It tracks both the group context and the individual sender's information.
type GroupMessage struct {
	gorm.Model
	// Group context
	GroupID   int64  `gorm:"not null;index" json:"group_id"`   // Group identifier from Telegram
	GroupName string `gorm:"type:text"      json:"group_name"` // Current name of the group

	// Message details
	UserID    int64     `gorm:"not null;index"     json:"user_id"`   // Message sender's identifier
	Message   string    `gorm:"not null;type:text" json:"message"`   // Content of the message
	Timestamp time.Time `gorm:"not null;index"     json:"timestamp"` // When the message was sent
}

// UserAnalysis represents a comprehensive behavioral and personality analysis
// for a specific user based on their chat interactions.
type UserAnalysis struct {
	gorm.Model
	// Analysis metadata
	UserID       int64     `gorm:"not null;index" json:"user_id"`       // User being analyzed
	Date         time.Time `gorm:"not null;index" json:"date"`          // When analysis was performed
	MessageCount int       `gorm:"not null"       json:"message_count"` // Number of messages analyzed

	// Analysis results
	CommunicationStyle string `gorm:"type:text" json:"communication_style"`  // How the user communicates
	PersonalityTraits  string `gorm:"type:text" json:"personality_traits"`   // Observed personality characteristics
	BehavioralPatterns string `gorm:"type:text" json:"behavioral_patterns"`  // Consistent behavior patterns
	WordChoicePatterns string `gorm:"type:text" json:"word_choice_patterns"` // Language and vocabulary usage
	InteractionHabits  string `gorm:"type:text" json:"interaction_habits"`   // How user engages with others
	UniqueQuirks       string `gorm:"type:text" json:"unique_quirks"`        // Distinctive characteristics
	EmotionalTriggers  string `gorm:"type:text" json:"emotional_triggers"`   // Topics causing emotional responses
}
