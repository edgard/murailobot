// Package model contains the domain entities and value objects for the MurailoBot application.
package model

// ChatBotInfo contains identification information about the chat bot itself.
// This information is used to properly handle the bot's own messages
// in conversation history and to create the bot's profile.
type ChatBotInfo struct {
	UserID      int64
	Username    string
	DisplayName string
}
