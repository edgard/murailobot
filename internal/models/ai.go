package models

// Request represents an AI request for generating a response.
// It includes the user message, recent conversation history, and user profiles.
type Request struct {
	UserID         int64
	Message        string
	RecentMessages []*Message
	UserProfiles   map[int64]*UserProfile
	SystemPrompt   string
	Temperature    float32
	MaxTokens      int
}

// Response represents the AI-generated response
type Response struct {
	Content      string
	Temperature  float32
	ModelName    string
	ErrorMessage string
}

// ProfileRequest represents a request to analyze user messages
// and generate or update a user profile.
type ProfileRequest struct {
	UserID          int64
	Messages        []*Message
	ExistingProfile *UserProfile
	SystemPrompt    string
}

// ProfileResponse represents the AI-generated user profile analysis
type ProfileResponse struct {
	Profile      *UserProfile
	ModelName    string
	ErrorMessage string
}
