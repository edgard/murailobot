// Package config provides configuration management for the Telegram bot application.
// It handles loading and validating configuration from multiple sources including
// default values, config files, and environment variables.
package config

import (
	"net/url"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/utils"
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

const componentName = "config"

// Config represents the complete application configuration.
// It contains all settings required for the bot's operation,
// including logging, AI, database, and Telegram configurations.
type Config struct {
	Log            LogConfig      `mapstructure:"log" validate:"required"`
	AI             AIConfig       `mapstructure:"ai" validate:"required"`
	Database       DatabaseConfig `mapstructure:"database" validate:"required"`
	Telegram       TelegramConfig `mapstructure:"telegram" validate:"required"`
	MaxMessageSize int            `mapstructure:"max_message_size" validate:"required,min=1,max=4096"` // Maximum message length in characters
}

// TelegramConfig holds all Telegram-related configuration settings.
// It includes authentication, security, message handling, and operational parameters.
type TelegramConfig struct {
	// Core settings for bot operation
	Token    string          `mapstructure:"token" validate:"required"`
	AdminID  int64           `mapstructure:"admin_id" validate:"required,gt=0,required_with=AllowedUserIDs"`
	Commands []CommandConfig `mapstructure:"commands" validate:"required,dive"`

	// Access control settings
	AllowedUserIDs []int64 `mapstructure:"allowed_user_ids" validate:"dive,gt=0,excluded_with=BlockedUserIDs"`
	BlockedUserIDs []int64 `mapstructure:"blocked_user_ids" validate:"dive,gt=0,nefield=AdminID"`

	// Bot response templates
	Messages BotMessages `mapstructure:"messages" validate:"required"`

	// Update polling configuration
	Polling PollingConfig `mapstructure:"polling" validate:"required"`

	// Chat interaction settings
	TypingInterval      time.Duration `mapstructure:"typing_interval" validate:"required,min=100ms,ltfield=TypingActionTimeout"`
	TypingActionTimeout time.Duration `mapstructure:"typing_action_timeout" validate:"required,min=1s,max=10s,ltfield=Polling.RequestTimeout"`

	// Operation timeouts
	DBOperationTimeout time.Duration `mapstructure:"db_operation_timeout" validate:"required,min=5s,max=60s"`
	AIRequestTimeout   time.Duration `mapstructure:"ai_request_timeout" validate:"required,min=1s,max=10m"`
}

// BotMessages defines templates for various bot responses.
// These messages are used to maintain consistent communication with users.
type BotMessages struct {
	Welcome        string `mapstructure:"welcome" validate:"required"`
	NotAuthorized  string `mapstructure:"not_authorized" validate:"required"`
	ProvideMessage string `mapstructure:"provide_message" validate:"required"`
	MessageTooLong string `mapstructure:"message_too_long" validate:"required"`
	AIError        string `mapstructure:"ai_error" validate:"required"`
	GeneralError   string `mapstructure:"general_error" validate:"required"`
	HistoryReset   string `mapstructure:"history_reset" validate:"required"`
}

// PollingConfig defines settings for Telegram's long polling mechanism.
type PollingConfig struct {
	Timeout            time.Duration `mapstructure:"timeout" validate:"required,min=1s,ltfield=RequestTimeout"`
	RequestTimeout     time.Duration `mapstructure:"request_timeout" validate:"required,min=1s"`
	DropPendingUpdates bool          `mapstructure:"drop_pending_updates"` // Whether to skip pending updates on startup
}

// LogConfig defines logging behavior and output format.
type LogConfig struct {
	Level  string `mapstructure:"level" validate:"required,oneof=debug info warn error"`
	Format string `mapstructure:"format" validate:"required,oneof=json text"`
}

// AIConfig defines settings for the AI service integration.
// It supports OpenAI-compatible APIs with customizable parameters.
type AIConfig struct {
	Token       string        `mapstructure:"token" validate:"required,ai_token"`
	BaseURL     string        `mapstructure:"base_url" validate:"required,url,startswith=https://,hostname_required"`
	Model       string        `mapstructure:"model" validate:"required,ai_model"`
	Temperature float32       `mapstructure:"temperature" validate:"required,min=0,max=2"`
	TopP        float32       `mapstructure:"top_p" validate:"required,min=0,max=1"`
	Instruction string        `mapstructure:"instruction" validate:"required,min=1"`
	Timeout     time.Duration `mapstructure:"timeout" validate:"required,min=1s,max=10m"`
}

// CommandConfig defines a bot command with its description.
type CommandConfig struct {
	Command     string `mapstructure:"command" validate:"required"`
	Description string `mapstructure:"description" validate:"required"`
}

// DatabaseConfig defines database connection and operation settings.
// Currently supports SQLite with specific optimizations.
type DatabaseConfig struct {
	Name            string        `mapstructure:"name" validate:"required"`
	MaxOpenConns    int           `mapstructure:"max_open_conns" validate:"required,min=1,max=100"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns" validate:"required,min=0,ltefield=MaxOpenConns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime" validate:"required,min=1s,max=24h"`
	MaxUsernameLen  int           `mapstructure:"max_username_len" validate:"required,min=1,max=256"`
	MaxHistoryLimit int           `mapstructure:"max_history_limit" validate:"required,min=1,max=100"`

	// SQLite-specific settings
	OperationTimeout     time.Duration `mapstructure:"operation_timeout" validate:"required,min=1s,max=30s"`
	LongOperationTimeout time.Duration `mapstructure:"long_operation_timeout" validate:"required,min=1s,max=60s"`

	// SQLite performance and reliability pragmas
	JournalMode string `mapstructure:"journal_mode" validate:"required,oneof=DELETE TRUNCATE PERSIST MEMORY WAL OFF"`
	Synchronous string `mapstructure:"synchronous" validate:"required,oneof=OFF NORMAL FULL EXTRA"`
	ForeignKeys bool   `mapstructure:"foreign_keys" validate:"required"`
	TempStore   string `mapstructure:"temp_store" validate:"required,oneof=DEFAULT FILE MEMORY"`
	CacheSizeKB int    `mapstructure:"cache_size_kb" validate:"required,min=1,max=10000"`
}

// Default configuration values for various components
const (
	// Log defaults
	DefaultLogLevel  = "info"
	DefaultLogFormat = "json"

	// Database defaults
	DefaultDBName            = "storage.db"
	DefaultDBMaxOpenConns    = 50
	DefaultDBMaxIdleConns    = 10
	DefaultDBMaxMessageSize  = 4096
	DefaultDBMaxUsernameLen  = 64
	DefaultDBMaxHistoryLimit = 50
	DefaultDBConnMaxLifetime = time.Hour

	// SQLite defaults for optimal performance
	DefaultDBOperationTimeout     = 5 * time.Second
	DefaultDBLongOperationTimeout = 30 * time.Second
	DefaultDBJournalMode          = "WAL"
	DefaultDBSynchronous          = "NORMAL"
	DefaultDBForeignKeys          = true
	DefaultDBTempStore            = "MEMORY"
	DefaultDBCacheSizeKB          = 2000

	// AI service defaults
	DefaultAIBaseURL     = "https://api.openai.com/v1"
	DefaultAIModel       = "gpt-4-turbo-preview"
	DefaultAITemperature = 0.5
	DefaultAITopP        = 0.9
	DefaultAITimeout     = 2 * time.Minute
	DefaultAIInstruction = "You are a helpful assistant focused on providing clear and accurate responses."
	DefaultAIMaxResponse = 4096

	// Telegram defaults
	DefaultTelegramMaxMessageLength    = 4096
	DefaultTelegramTypingInterval      = 3 * time.Second
	DefaultTelegramTypingActionTimeout = 5 * time.Second
	DefaultTelegramDBOperationTimeout  = 15 * time.Second
	DefaultTelegramAIRequestTimeout    = 2 * time.Minute
	DefaultTelegramPollingTimeout      = 10 * time.Second
	DefaultTelegramRequestTimeout      = 30 * time.Second
	DefaultTelegramMaxRoutines         = 50 // Limited by CPU count + 2
	DefaultTelegramDropPendingUpdates  = true
)

// DefaultBotMessages provides default templates for bot responses.
var DefaultBotMessages = BotMessages{
	Welcome:        "👋 Welcome! I'm ready to assist you. Use /mrl followed by your message to start a conversation.",
	NotAuthorized:  "🚫 Access denied. Please contact the administrator.",
	HistoryReset:   "🔄 Chat history has been cleared.",
	ProvideMessage: "ℹ️ Please provide a message with your command.",
	GeneralError:   "❌ An error occurred. Please try again later.",
	AIError:        "🤖 Unable to process request. Please try again.",
	MessageTooLong: "📝 Message exceeds maximum length of %d characters.",
}

// DefaultBotCommands defines the standard set of bot commands.
var DefaultBotCommands = []CommandConfig{
	{Command: "start", Description: "Start conversation with the bot"},
	{Command: "mrl", Description: "Generate AI response"},
	{Command: "mrl_reset", Description: "Reset chat history (admin only)"},
}

// Load reads configuration from multiple sources in order of precedence:
// 1. Default values
// 2. config.yaml file
// 3. BOT_* environment variables
// It returns a validated configuration or an error if validation fails.
func Load() (*Config, error) {
	setDefaults()

	utils.WriteDebugLog(componentName, "default configuration set",
		utils.KeyAction, "set_defaults")

	cfg := &Config{}

	if err := loadConfig(); err != nil {
		return nil, utils.NewError(componentName, utils.ErrInvalidConfig, "failed to load config file", utils.CategoryConfig, err)
	}

	if err := viper.Unmarshal(cfg); err != nil {
		return nil, utils.NewError(componentName, utils.ErrInvalidConfig, "failed to parse config", utils.CategoryConfig, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, utils.NewError(componentName, utils.ErrInvalidConfig, "validation failed", utils.CategoryConfig, err)
	}

	utils.WriteInfoLog(componentName, "configuration loaded successfully",
		utils.KeyAction, "load_config",
		"config_summary", map[string]interface{}{
			"log_level":        cfg.Log.Level,
			"log_format":       cfg.Log.Format,
			"max_message_size": cfg.MaxMessageSize,
			"ai_model":         cfg.AI.Model,
			"db_name":          cfg.Database.Name,
		})

	return cfg, nil
}

// loadConfig initializes viper and loads configuration from file and environment.
func loadConfig() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	viper.SetEnvPrefix("BOT")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.BindEnv("telegram.token", "BOT_TELEGRAM_TOKEN"); err != nil {
		return utils.NewError(componentName, utils.ErrInvalidConfig, "failed to bind telegram token env var", utils.CategoryConfig, err)
	}
	if err := viper.BindEnv("telegram.admin_id", "BOT_TELEGRAM_ADMIN_ID"); err != nil {
		return utils.NewError(componentName, utils.ErrInvalidConfig, "failed to bind telegram admin ID env var", utils.CategoryConfig, err)
	}
	if err := viper.BindEnv("ai.token", "BOT_AI_TOKEN"); err != nil {
		return utils.NewError(componentName, utils.ErrInvalidConfig, "failed to bind AI token env var", utils.CategoryConfig, err)
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return utils.NewError(componentName, utils.ErrInvalidConfig, "failed to read config file", utils.CategoryConfig, err)
		}
	}

	return nil
}

// setDefaults initializes default values for all optional configuration parameters.
func setDefaults() {
	defaults := map[string]interface{}{
		"max_message_size": DefaultTelegramMaxMessageLength,

		"log": map[string]interface{}{
			"level":  DefaultLogLevel,
			"format": DefaultLogFormat,
		},
		"database": map[string]interface{}{
			"name":                   DefaultDBName,
			"max_open_conns":         DefaultDBMaxOpenConns,
			"max_idle_conns":         DefaultDBMaxIdleConns,
			"conn_max_lifetime":      DefaultDBConnMaxLifetime,
			"max_username_len":       DefaultDBMaxUsernameLen,
			"max_history_limit":      DefaultDBMaxHistoryLimit,
			"operation_timeout":      DefaultDBOperationTimeout,
			"long_operation_timeout": DefaultDBLongOperationTimeout,
			"journal_mode":           DefaultDBJournalMode,
			"synchronous":            DefaultDBSynchronous,
			"foreign_keys":           DefaultDBForeignKeys,
			"temp_store":             DefaultDBTempStore,
			"cache_size_kb":          DefaultDBCacheSizeKB,
		},
		"ai": map[string]interface{}{
			"base_url":    DefaultAIBaseURL,
			"model":       DefaultAIModel,
			"temperature": DefaultAITemperature,
			"top_p":       DefaultAITopP,
			"timeout":     DefaultAITimeout,
			"instruction": DefaultAIInstruction,
		},
		"telegram": map[string]interface{}{
			"allowed_user_ids":      []int64{},
			"blocked_user_ids":      []int64{},
			"typing_interval":       DefaultTelegramTypingInterval,
			"typing_action_timeout": DefaultTelegramTypingActionTimeout,
			"db_operation_timeout":  DefaultTelegramDBOperationTimeout,
			"ai_request_timeout":    DefaultTelegramAIRequestTimeout,
			"messages":              DefaultBotMessages,
			"commands":              DefaultBotCommands,
			"polling": map[string]interface{}{
				"timeout":              DefaultTelegramPollingTimeout,
				"request_timeout":      DefaultTelegramRequestTimeout,
				"drop_pending_updates": DefaultTelegramDropPendingUpdates,
			},
		},
	}

	for key, value := range defaults {
		viper.SetDefault(key, value)
	}
}

// Validate performs comprehensive validation of all configuration fields
// using struct tags and custom validation rules.
func (c *Config) Validate() error {
	v := validator.New()

	if err := v.RegisterValidation("ai_token", func(fl validator.FieldLevel) bool {
		return strings.HasPrefix(fl.Field().String(), "sk-")
	}); err != nil {
		return utils.NewError(componentName, utils.ErrValidation, "failed to register AI token validator", utils.CategoryValidation, err)
	}

	if err := v.RegisterValidation("ai_model", func(fl validator.FieldLevel) bool {
		model := fl.Field().String()
		return len(model) > 0
	}); err != nil {
		return utils.NewError(componentName, utils.ErrValidation, "failed to register AI model validator", utils.CategoryValidation, err)
	}

	if err := v.RegisterValidation("hostname_required", func(fl validator.FieldLevel) bool {
		urlStr := fl.Field().String()
		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			return false
		}
		return parsedURL.Hostname() != ""
	}); err != nil {
		return utils.NewError(componentName, utils.ErrValidation, "failed to register hostname validator", utils.CategoryValidation, err)
	}

	if err := v.Struct(c); err != nil {
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			var msg string
			for i, e := range validationErrors {
				if i > 0 {
					msg += ", "
				}
				msg += e.Field() + ": " + e.Tag()
			}
			return utils.Errorf(componentName, utils.ErrValidation, utils.CategoryValidation,
				"validation errors: %s", msg)
		}
		return utils.NewError(componentName, utils.ErrValidation, "validation failed", utils.CategoryValidation, err)
	}

	return nil
}
