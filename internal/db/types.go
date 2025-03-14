// Package db provides storage functionality for chat conversations and analytics.
// It currently implements a SQLite-based storage backend with support for:
// - Chat history management
// - Group message tracking
// - User profiles
// - Context-aware operations
package db

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Database configuration constants.
const (
	defaultMaxOpenConn = 1 // Maximum number of open connections
)

// Database defines the interface for group messages and user profiles operations.
type Database interface {
	// SaveGroupMessage stores a message from a group chat.
	// The groupName is stored to track group name changes over time.
	// It returns an error if the storage operation fails.
	SaveGroupMessage(groupID int64, groupName string, userID int64, message string) error

	// GetRecentGroupMessages retrieves the most recent group messages from a specific group chat.
	// Messages are returned in reverse chronological order (newest first).
	GetRecentGroupMessages(groupID int64, limit int) ([]GroupMessage, error)

	// GetGroupMessagesInTimeRange retrieves group messages between start and end times.
	// Times should be in UTC to ensure consistent queries across timezones.
	GetGroupMessagesInTimeRange(start, end time.Time) ([]GroupMessage, error)

	// GetAllGroupMessages retrieves all group messages stored in the database.
	// This can be a potentially large dataset, so handle with care.
	GetAllGroupMessages() ([]GroupMessage, error)

	// GetUnprocessedGroupMessages retrieves all group messages that have not been processed yet
	// (have a nil ProcessedAt field).
	GetUnprocessedGroupMessages() ([]GroupMessage, error)

	// MarkGroupMessagesAsProcessed marks a batch of group messages as processed
	// by setting their ProcessedAt timestamp to the current time.
	// This operation is used to track which messages have been analyzed for profiles.
	MarkGroupMessagesAsProcessed(messageIDs []uint) error

	// DeleteProcessedGroupMessages deletes all group messages that have been processed
	// (have a non-nil ProcessedAt field) and were processed before the given cutoff time.
	// This provides a safety window where messages remain available for re-processing if needed.
	DeleteProcessedGroupMessages(cutoffTime time.Time) error

	// DeleteProcessedGroupMessagesExcept deletes processed messages for a specific group chat
	// that were processed before the cutoff time, except those with IDs in the preserveIDs list.
	// This allows preserving recent messages for context.
	DeleteProcessedGroupMessagesExcept(groupID int64, cutoffTime time.Time, preserveIDs []uint) error

	// GetUniqueGroupChats returns the IDs of all distinct group chats in the database.
	GetUniqueGroupChats() ([]int64, error)

	// GetUserProfile retrieves a user's profile by user ID.
	// Returns nil and no error if the profile does not exist.
	GetUserProfile(userID int64) (*UserProfile, error)

	// SaveUserProfile creates or updates a user profile.
	SaveUserProfile(profile *UserProfile) error

	// GetAllUserProfiles retrieves all user profiles.
	GetAllUserProfiles() (map[int64]*UserProfile, error)

	// DeleteAll removes all stored data, including group messages and user profiles.
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

	// Processing tracking
	ProcessedAt *time.Time `gorm:"index" json:"processed_at"` // When this message was processed for profiles
}

// UserProfile represents a user's accumulated profile information
// built from ongoing analysis of their messages.
type UserProfile struct {
	gorm.Model
	UserID          int64     `gorm:"not null;uniqueIndex" json:"user_id"`          // User identifier from Telegram
	DisplayNames    string    `gorm:"type:text"            json:"display_names"`    // Known names/nicknames (comma-separated)
	OriginLocation  string    `gorm:"type:text"            json:"origin_location"`  // Where the user is from
	CurrentLocation string    `gorm:"type:text"            json:"current_location"` // Where the user currently lives
	AgeRange        string    `gorm:"type:text"            json:"age_range"`        // Approximate age range
	Traits          string    `gorm:"type:text"            json:"traits"`           // Personality traits and characteristics
	LastUpdated     time.Time `gorm:"not null"             json:"last_updated"`     // When profile was last updated
}

// Format: "UID [user_id] ([display_names]) | [origin_location] | [current_location] | [age_range] | [traits]".
func (p *UserProfile) FormatPipeDelimited() string {
	if p == nil {
		return "Error: nil profile"
	}

	// Use empty string placeholder for any nil fields
	displayNames := p.DisplayNames
	if displayNames == "" {
		displayNames = "Unknown"
	}

	originLocation := p.OriginLocation
	if originLocation == "" {
		originLocation = "Unknown"
	}

	currentLocation := p.CurrentLocation
	if currentLocation == "" {
		currentLocation = "Unknown"
	}

	ageRange := p.AgeRange
	if ageRange == "" {
		ageRange = "Unknown"
	}

	traits := p.Traits
	if traits == "" {
		traits = "Unknown"
	}

	return fmt.Sprintf("UID %d (%s) | %s | %s | %s | %s",
		p.UserID,
		displayNames,
		originLocation,
		currentLocation,
		ageRange,
		traits)
}
