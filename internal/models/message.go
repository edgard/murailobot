package models

import (
	"time"
)

// Message represents a message sent in a Telegram group chat.
// It stores the message content, sender information, and processing status
// for use in conversation context and user profile analysis.
type Message struct {
	ID        uint       `gorm:"primarykey" json:"id"`
	CreatedAt time.Time  `gorm:"index" json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"deleted_at,omitempty"`

	GroupID   int64  `gorm:"not null;index" json:"group_id"`
	GroupName string `gorm:"type:text" json:"group_name"`

	UserID    int64  `gorm:"not null;index" json:"user_id"`
	Content   string `gorm:"not null;type:text" json:"content"`
	IsFromBot bool   `gorm:"not null;index" json:"is_from_bot"`
	Processed bool   `gorm:"not null;index" json:"processed"`

	// ProcessedAt marks when the message was analyzed for user profiling
	ProcessedAt *time.Time `gorm:"index" json:"processed_at,omitempty"`
}
