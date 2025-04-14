// Package model contains the core domain entities for the MurailoBot application.
// These models represent the core business objects and are independent of external concerns.
package model

import (
	"fmt"
	"time"
)

// UserProfile represents a user's accumulated profile information
// derived from their message history. It stores demographic information,
// personality traits, and other characteristics identified through
// AI analysis of the user's messages.
type UserProfile struct {
	ID        uint
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time

	UserID          int64
	DisplayNames    string
	OriginLocation  string
	CurrentLocation string
	AgeRange        string
	Traits          string
	LastUpdated     time.Time
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
