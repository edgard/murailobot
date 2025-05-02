package database

import (
	"database/sql"
	"time"
)

// Message represents a chat message stored in the database.
// It contains both the message content and metadata.
type Message struct {
	ID          uint         `db:"id"`
	ChatID      int64        `db:"chat_id"`
	UserID      int64        `db:"user_id"`
	Content     string       `db:"content"`
	Timestamp   time.Time    `db:"timestamp"`
	CreatedAt   time.Time    `db:"created_at"`
	UpdatedAt   time.Time    `db:"updated_at"`
	ProcessedAt sql.NullTime `db:"processed_at"`
}

// UserProfile stores information about a Telegram user based on their messages.
// It's populated through AI analysis of user interactions and can be edited by admins.
type UserProfile struct {
	ID              uint      `db:"id"`
	UserID          int64     `db:"user_id"`
	Aliases         string    `db:"aliases"`
	OriginLocation  string    `db:"origin_location"`
	CurrentLocation string    `db:"current_location"`
	AgeRange        string    `db:"age_range"`
	Traits          string    `db:"traits"`
	CreatedAt       time.Time `db:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"`
}
