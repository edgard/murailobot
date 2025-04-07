package models

import (
	"fmt"
	"time"
)

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
	IsBot           bool      `gorm:"not null;default:false" json:"is_bot"`
	Username        string    `gorm:"type:text"            json:"username,omitempty"`
}

// FormatPipeDelimited formats the user profile as a pipe-delimited string
// for display purposes. The format follows:
// "UID [user_id] ([display_names]) | [origin_location] | [current_location] | [age_range] | [traits]"
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
