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

	ChatID    int64     `db:"chat_id"` // Column name now matches field name
	UserID    int64     `db:"user_id"`
	Content   string    `db:"content"`
	Timestamp time.Time `db:"timestamp"`

	ProcessedAt sql.NullTime `db:"processed_at"` // Use sql.NullTime for nullable timestamps
}

// UserProfile represents a user's accumulated profile information
// derived from their message history. It stores demographic information,
// personality traits, and other characteristics identified through
// AI analysis of the user's messages.
type UserProfile struct {
	ID        uint      `db:"id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`

	UserID          int64  `db:"user_id"`
	Aliases         string `db:"aliases"`
	OriginLocation  string `db:"origin_location"`
	CurrentLocation string `db:"current_location"`
	AgeRange        string `db:"age_range"`
	Traits          string `db:"traits"`
}
