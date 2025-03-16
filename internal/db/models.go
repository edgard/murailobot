// Package db provides database models and operations for MurailoBot,
// handling message storage, user profile management, and data persistence.
package db

import (
	"fmt"
	"time"
)

// Message represents a message sent in a Telegram group chat.
// It stores the message content, sender information, and processing status
// for use in conversation context and user profile analysis.
type Message struct {
	ID        uint       `gorm:"primarykey" json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index"      json:"deleted_at,omitempty"`

	GroupID   int64  `gorm:"not null;index" json:"group_id"`
	GroupName string `gorm:"type:text"      json:"group_name"`

	UserID    int64     `gorm:"not null;index"     json:"user_id"`
	Content   string    `gorm:"not null;type:text" json:"content"`
	Timestamp time.Time `gorm:"not null;index"     json:"timestamp"`

	ProcessedAt *time.Time `gorm:"index" json:"processed_at"`
}

// UserProfile represents a user's accumulated profile information
// derived from their message history. It stores demographic information,
// personality traits, and other characteristics identified through
// AI analysis of the user's messages.
type UserProfile struct {
	ID        uint       `gorm:"primarykey" json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index"      json:"deleted_at,omitempty"`

	UserID          int64     `gorm:"not null;uniqueIndex" json:"user_id"`
	DisplayNames    string    `gorm:"type:text"            json:"display_names"`
	OriginLocation  string    `gorm:"type:text"            json:"origin_location"`
	CurrentLocation string    `gorm:"type:text"            json:"current_location"`
	AgeRange        string    `gorm:"type:text"            json:"age_range"`
	Traits          string    `gorm:"type:text"            json:"traits"`
	LastUpdated     time.Time `gorm:"not null"             json:"last_updated"`
}

// FormatPipeDelimited formats the user profile as a pipe-delimited string
// for display purposes. The format follows:
// "UID [user_id] ([display_names]) | [origin_location] | [current_location] | [age_range] | [traits]"
//
// Empty fields are replaced with "Unknown" for consistent formatting.
func (p *UserProfile) FormatPipeDelimited() string {
	if p == nil {
		return "Error: nil profile"
	}

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
