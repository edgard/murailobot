// Package config manages application configuration from environment variables,
// config files, and default values.
package config

import (
	"errors"
	"time"
)

const (
	defaultAITimeout = 2 * time.Minute
)

var ErrValidation = errors.New("validation error")

var defaults = map[string]any{
	"ai.base_url":    "https://api.openai.com/v1",
	"ai.model":       "gpt-4",
	"ai.temperature": 1.0,
	"ai.instruction": "You are a helpful assistant focused on providing clear and accurate responses.",
	"ai.timeout":     defaultAITimeout,

	"telegram.messages.welcome":         "👋 Welcome! I'm ready to assist you. Use /mrl followed by your message to start a conversation.",
	"telegram.messages.not_authorized":  "🚫 Access denied. Please contact the administrator.",
	"telegram.messages.provide_message": "ℹ️ Please provide a message with your command.",
	"telegram.messages.general_error":   "❌ An error occurred. Please try again later.",
	"telegram.messages.history_reset":   "🔄 Chat history has been cleared.",
	"telegram.messages.timeout":         "⏱️ Request timed out. Please try again later.",

	"log.level":  "info",
	"log.format": "json",
}

// Config defines the application configuration. Values can be set via environment
// variables prefixed with BOT_ (e.g., BOT_AI_TOKEN) or through config.yaml.
type Config struct {
	// AI service configuration
	AIToken       string        `koanf:"ai.token"       validate:"required"`
	AIBaseURL     string        `koanf:"ai.base_url"    validate:"required,url"`
	AIModel       string        `koanf:"ai.model"       validate:"required"`
	AITemperature float32       `koanf:"ai.temperature" validate:"required,min=0,max=2"`
	AIInstruction string        `koanf:"ai.instruction" validate:"required"`
	AITimeout     time.Duration `koanf:"ai.timeout"     validate:"required,min=1s,max=10m"`

	// Telegram settings
	TelegramToken                string `koanf:"telegram.token"                    validate:"required"`
	TelegramAdminID              int64  `koanf:"telegram.admin_id"                 validate:"required,gt=0"`
	TelegramWelcomeMessage       string `koanf:"telegram.messages.welcome"         validate:"required"`
	TelegramNotAuthorizedMessage string `koanf:"telegram.messages.not_authorized"  validate:"required"`
	TelegramProvideMessage       string `koanf:"telegram.messages.provide_message" validate:"required"`
	TelegramGeneralErrorMessage  string `koanf:"telegram.messages.general_error"   validate:"required"`
	TelegramHistoryResetMessage  string `koanf:"telegram.messages.history_reset"   validate:"required"`
	TelegramTimeoutMessage       string `koanf:"telegram.messages.timeout"         validate:"required"`

	// Logging settings
	LogLevel  string `koanf:"log.level"  validate:"required,oneof=debug info warn error"`
	LogFormat string `koanf:"log.format" validate:"required,oneof=json text"`
}
