package config

import "time"

// Default values for configuration
const (
	// Log defaults
	DefaultLogLevel  = "info"
	DefaultLogFormat = "json"

	// Database defaults
	DefaultDBName            = "storage.db" // Default SQLite database name
	DefaultDBMaxOpenConns    = 50
	DefaultDBMaxIdleConns    = 10
	DefaultDBMaxMessageSize  = 4096 // Maximum message size for database storage
	DefaultDBConnMaxLifetime = time.Hour

	// OpenAI defaults
	DefaultOpenAIBaseURL     = "https://api.openai.com/v1"
	DefaultOpenAIModel       = "gpt-4-turbo-preview"
	DefaultOpenAITemperature = 0.5
	DefaultOpenAITopP        = 0.9
	DefaultOpenAITimeout     = 2 * time.Minute
	DefaultOpenAIInstruction = "You are a helpful assistant focused on providing clear and accurate responses."

	// Bot defaults
	DefaultBotMaxMessageLength    = 4096             // Telegram's maximum message length
	DefaultBotTypingInterval      = 3 * time.Second  // How often to refresh typing indicator
	DefaultBotPollTimeout         = 5 * time.Second  // How long to wait for updates
	DefaultBotRequestTimeout      = 30 * time.Second // General API request timeout
	DefaultBotMaxRoutines         = 50               // Maximum concurrent handlers
	DefaultBotDropPendingUpdates  = true             // Clean start on bot launch
	DefaultBotTypingActionTimeout = 3 * time.Second  // Quick typing indicator response
	DefaultBotDBOperationTimeout  = 15 * time.Second // Balanced DB operation timeout
	DefaultBotAIRequestTimeout    = 2 * time.Minute  // Match OpenAI timeout for consistent behavior
)

// Default bot messages
var DefaultBotMessages = BotMessages{
	Welcome:        "👋 Welcome! I'm ready to assist you. Use /mrl followed by your message to start a conversation.",
	NotAuthorized:  "🚫 Access denied. Please contact the administrator.",
	HistoryReset:   "🔄 Chat history has been cleared.",
	ProvideMessage: "ℹ️ Please provide a message with your command.",
	GeneralError:   "❌ An error occurred. Please try again later.",
	AIError:        "🤖 Unable to process request. Please try again.",
	MessageTooLong: "📝 Message exceeds maximum length of %d characters.",
}

// Default bot commands
var DefaultBotCommands = []CommandConfig{
	{Command: "start", Description: "Start conversation with the bot"},
	{Command: "mrl", Description: "Generate response using OpenAI"},
	{Command: "mrl_reset", Description: "Reset chat history (admin only)"},
}
