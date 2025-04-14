// Package model contains the domain entities and value objects for the MurailoBot application.
package model

// AIRequest contains all the information needed to generate an AI response.
// It includes the user's message, conversation context, and user profiles
// to help create more personalized and contextually appropriate responses.
type AIRequest struct {
	UserID         int64
	Message        string
	RecentMessages []*Message
	UserProfiles   map[int64]*UserProfile
}
