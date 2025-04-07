package models

// BotInfo contains bot identification information used by services
// to identify and represent the bot in conversations. It provides
// the core identity details needed for the bot to operate in
// Telegram group chats and to be properly represented in AI prompts.
type BotInfo struct {
	ID        int64  // Telegram's unique identifier for the bot
	UserName  string // Bot's username without @ symbol
	FirstName string // Bot's display name in Telegram
}
