// Package telegram implements a Telegram bot that integrates with OpenAI
// for generating AI-powered responses and managing conversation history.
package telegram

import (
	"errors"
	"time"

	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/openai"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Default telegram bot configuration values.
const (
	defaultUpdateOffset   = 0                // Initial offset for update polling
	defaultUpdateTimeout  = 60               // Timeout for long polling updates (seconds)
	defaultTypingInterval = 5 * time.Second  // Interval for sending typing indicators
	apiOperationTimeout   = 15 * time.Second // Default timeout for API operations
)

// Error definitions for common error conditions.
var (
	ErrNilConfig        = errors.New("config is nil")         // Configuration not provided
	ErrNilDatabase      = errors.New("database is nil")       // Database not initialized
	ErrNilOpenAIService = errors.New("OpenAI service is nil") // OpenAI service not provided
	ErrNilMessage       = errors.New("message is nil")        // Message object is nil
	ErrUnauthorized     = errors.New("unauthorized access")   // User not authorized
)

// Messages defines configurable bot response templates for various states.
type Messages struct {
	Welcome      string // Initial greeting message
	Unauthorized string // Access denied message
	Provide      string // Prompt for providing a message
	OpenAIError  string // OpenAI-specific error message
	GeneralError string // Generic error message
	HistoryReset string // History cleared confirmation
	Timeout      string // Request timeout message
}

// Config holds bot configuration including authentication and response messages.
type Config struct {
	Token    string   // Telegram Bot API token
	AdminID  int64    // Administrator's Telegram user ID
	Messages Messages // Configurable response templates
}

// Bot implements a Telegram bot with OpenAI capabilities, handling message
// processing and user interactions through commands.
type Bot struct {
	api    *tgbotapi.BotAPI // Telegram Bot API client
	db     db.Database      // Database for conversation history
	openAI openai.Service   // OpenAI service for generating responses
	cfg    *Config          // Bot configuration
}
