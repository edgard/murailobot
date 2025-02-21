package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
)

// Common errors
var (
	ErrConfiguration = errors.New("configuration error")
	ErrValidation    = errors.New("validation error")
)

// Config represents the complete application configuration
type Config struct {
	Log      LogConfig      `mapstructure:"log" validate:"required"`
	OpenAI   OpenAIConfig   `mapstructure:"openai" validate:"required"`
	Bot      BotConfig      `mapstructure:"bot" validate:"required"`
	Database DatabaseConfig `mapstructure:"database" validate:"required"`
}

// LogConfig defines logging-related configuration
type LogConfig struct {
	Level  string `mapstructure:"level" validate:"required,oneof=debug info warn error"`
	Format string `mapstructure:"format" validate:"required,oneof=json text"`
}

// OpenAIConfig defines OpenAI API configuration
type OpenAIConfig struct {
	Token       string        `mapstructure:"token" validate:"required"`
	BaseURL     string        `mapstructure:"base_url" validate:"required,url,startswith=https://"`
	Model       string        `mapstructure:"model" validate:"required"`
	Temperature float32       `mapstructure:"temperature" validate:"required,min=0,max=2"`
	TopP        float32       `mapstructure:"top_p" validate:"required,min=0,max=1"`
	Instruction string        `mapstructure:"instruction" validate:"required"`
	Timeout     time.Duration `mapstructure:"timeout" validate:"required,min=1s,max=10m"`
}

// BotConfig defines bot-related configuration
type BotConfig struct {
	Token              string          `mapstructure:"token" validate:"required"`
	AdminUID           int64           `mapstructure:"admin_uid" validate:"required,gt=0"`
	MaxMessageLength   int             `mapstructure:"max_message_length" validate:"required,min=1,max=4096"`
	AllowedUserIDs     []int64         `mapstructure:"allowed_user_ids" validate:"dive,gt=0"`
	BlockedUserIDs     []int64         `mapstructure:"blocked_user_ids" validate:"dive,gt=0"`
	TypingInterval     time.Duration   `mapstructure:"typing_interval" validate:"required,min=100ms"`
	PollTimeout        time.Duration   `mapstructure:"poll_timeout" validate:"required,min=1s"`
	RequestTimeout     time.Duration   `mapstructure:"request_timeout" validate:"required,min=1s"`
	MaxRoutines        int             `mapstructure:"max_routines" validate:"required,min=1"`
	DropPendingUpdates bool            `mapstructure:"drop_pending_updates"`
	Commands           []CommandConfig `mapstructure:"commands" validate:"required,dive"`
	Messages           BotMessages     `mapstructure:"messages" validate:"required"`

	// Operation timeouts
	TypingActionTimeout time.Duration `mapstructure:"typing_action_timeout" validate:"required,min=1s,max=10s"`
	DBOperationTimeout  time.Duration `mapstructure:"db_operation_timeout" validate:"required,min=5s,max=60s"`
	AIRequestTimeout    time.Duration `mapstructure:"ai_request_timeout" validate:"required,min=1s,max=10m"`
}

// Validate performs custom validation for BotConfig
func (c *BotConfig) Validate() error {
	// Check for blocked admin
	for _, id := range c.BlockedUserIDs {
		if id == c.AdminUID {
			return fmt.Errorf("%w: admin cannot be blocked", ErrValidation)
		}
	}

	// Check for overlap between allowed and blocked users
	allowedMap := make(map[int64]bool)
	for _, id := range c.AllowedUserIDs {
		allowedMap[id] = true
	}

	for _, id := range c.BlockedUserIDs {
		if allowedMap[id] {
			return fmt.Errorf("%w: user cannot be both allowed and blocked", ErrValidation)
		}
	}

	return nil
}

// CommandConfig defines a bot command
type CommandConfig struct {
	Command     string `mapstructure:"command" validate:"required"`
	Description string `mapstructure:"description" validate:"required"`
}

// BotMessages defines bot response messages
type BotMessages struct {
	Welcome        string `mapstructure:"welcome" validate:"required"`
	NotAuthorized  string `mapstructure:"not_authorized" validate:"required"`
	ProvideMessage string `mapstructure:"provide_message" validate:"required"`
	MessageTooLong string `mapstructure:"message_too_long" validate:"required"`
	AIError        string `mapstructure:"ai_error" validate:"required"`
	GeneralError   string `mapstructure:"general_error" validate:"required"`
	HistoryReset   string `mapstructure:"history_reset" validate:"required"`
}

// DatabaseConfig defines database connection configuration
type DatabaseConfig struct {
	Name            string        `mapstructure:"name" validate:"required"`
	MaxOpenConns    int           `mapstructure:"max_open_conns" validate:"required,min=1,max=100"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns" validate:"required,min=0,ltefield=MaxOpenConns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime" validate:"required,min=1s,max=24h"`
	MaxMessageSize  int           `mapstructure:"max_message_size" validate:"required,min=1,max=4096"`
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validate performs validation of all configuration fields
func (c *Config) Validate() error {
	v := validator.New()

	// Validate using struct tags
	if err := v.Struct(c); err != nil {
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			var errMsgs []string
			for _, e := range validationErrors {
				errMsgs = append(errMsgs, fmt.Sprintf("%s: %s", e.Field(), e.Tag()))
			}
			return fmt.Errorf("%w: %v", ErrValidation, errMsgs)
		}
		return fmt.Errorf("%w: validation failed: %v", ErrValidation, err)
	}

	// Run custom validations
	if err := c.Bot.Validate(); err != nil {
		return fmt.Errorf("bot config validation failed: %w", err)
	}

	return nil
}
