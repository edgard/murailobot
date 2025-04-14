// Package model contains the core domain entities for the MurailoBot application.
// These models represent the core business objects and are independent of external concerns.
package model

import (
	"time"
)

// Message represents a message sent in a Telegram group chat.
// It stores the message content, sender information, and processing status
// for use in conversation context and user profile analysis.
type Message struct {
	ID        uint
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time

	GroupID   int64
	GroupName string

	UserID    int64
	Content   string
	Timestamp time.Time

	ProcessedAt *time.Time
}
