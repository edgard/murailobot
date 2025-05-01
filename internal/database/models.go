// Package database provides database setup, models, and data access layer (Store).
package database

import (
	"database/sql"
	"time"
)

// Message represents a message sent in a Telegram group chat.
// It stores the message content, sender information, and processing status
// for use in conversation context and user profile analysis.
type Message struct {
	ID        uint      `db:"id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`

	ChatID    int64     `db:"chat_id"`   // Telegram Chat ID
	UserID    int64     `db:"user_id"`   // Telegram User ID of the sender
	Content   string    `db:"content"`   // Text content of the message
	Timestamp time.Time `db:"timestamp"` // Time the message was sent

	ProcessedAt sql.NullTime `db:"processed_at"` // Timestamp when the message was processed for profile analysis (NULL if not processed)
}

// UserProfile represents a user's accumulated profile information
// derived from their message history. It stores demographic information,
// personality traits, and other characteristics identified through
// AI analysis of the user's messages.
type UserProfile struct {
	ID        uint      `db:"id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`

	UserID          int64  `db:"user_id"`          // Unique Telegram User ID
	Aliases         string `db:"aliases"`          // Comma-separated list of known aliases or names
	OriginLocation  string `db:"origin_location"`  // User's place of origin, if known
	CurrentLocation string `db:"current_location"` // User's current location, if known
	AgeRange        string `db:"age_range"`        // Estimated age range (e.g., "20-30")
	Traits          string `db:"traits"`           // Comma-separated list of observed personality traits or characteristics
}
