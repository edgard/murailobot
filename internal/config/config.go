// Package config handles loading and validation of application configuration.
package config

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/go-telegram/bot/models"
	"github.com/spf13/viper"
)

// Config holds the application configuration.
// Tags use mapstructure for Viper unmarshalling and validate for validation.
type Config struct {
	Telegram  TelegramConfig    `mapstructure:"telegram" validate:"required"`
	Database  DatabaseConfig    `mapstructure:"database" validate:"required"`
	Gemini    GeminiConfig      `mapstructure:"gemini" validate:"required"`
	Logger    LoggerConfig      `mapstructure:"logger" validate:"required"`
	Scheduler SchedulerConfig   `mapstructure:"scheduler" validate:"required"`
	Messages  BotMessagesConfig `mapstructure:"messages" validate:"required"`
}

// TelegramConfig holds Telegram specific settings.
type TelegramConfig struct {
	Token       string       `mapstructure:"token" validate:"required,min=45,max=50"` // Bot token from BotFather (typically 46 chars)
	AdminUserID int64        `mapstructure:"admin_user_id" validate:"required,gt=0"`  // User ID of the bot administrator
	BotInfo     *models.User `mapstructure:"-" validate:"-"`                          // Bot information (filled in at runtime)
}

// DatabaseConfig holds database connection details.
type DatabaseConfig struct {
	Path               string `mapstructure:"path" validate:"required,filepath"`             // Database file path (e.g., "./storage.db")
	MaxHistoryMessages int    `mapstructure:"max_history_messages" validate:"required,gt=0"` // Maximum number of historical messages to include in context
}

// GeminiConfig holds Google Gemini API settings.
type GeminiConfig struct {
	APIKey            string  `mapstructure:"api_key" validate:"required,min=10"`          // Gemini API Key
	ModelName         string  `mapstructure:"model_name" validate:"required"`              // Gemini Model Name (e.g., "gemini-1.5-flash")
	Temperature       float32 `mapstructure:"temperature" validate:"required,gte=0,lte=2"` // Sampling temperature for generation
	SearchGrounding   bool    `mapstructure:"search_grounding"`                            // Enable Google Search grounding tool
	SystemInstruction string  `mapstructure:"system_instruction" validate:"required"`      // Developer-provided system instruction for AI
}

// LoggerConfig holds logging settings.
type LoggerConfig struct {
	Level string `mapstructure:"level" validate:"required,oneof=debug info warn error"` // Log level
	JSON  bool   `mapstructure:"json"`                                                  // JSON output flag
}

// SchedulerConfig holds settings for scheduled tasks.
type SchedulerConfig struct {
	Tasks map[string]TaskConfig `mapstructure:"tasks" validate:"required,dive"`
}

// TaskConfig defines settings for a single scheduled task.
type TaskConfig struct {
	Schedule string `mapstructure:"schedule" validate:"required,cron"`
	Enabled  bool   `mapstructure:"enabled"`
}

// BotMessagesConfig holds configurable strings used by the bot in replies.
type BotMessagesConfig struct {
	// General
	Welcome       string `mapstructure:"welcome" validate:"required"`
	GeneralError  string `mapstructure:"general_error" validate:"required"`
	NotAuthorized string `mapstructure:"not_authorized" validate:"required"`
	Help          string `mapstructure:"help" validate:"required"`

	// Admin Command Responses
	HistoryReset            string `mapstructure:"history_reset" validate:"required"`
	Analyzing               string `mapstructure:"analyzing" validate:"required"`
	AnalysisNoMessages      string `mapstructure:"analysis_no_messages" validate:"required"`
	AnalysisCompleteFmt     string `mapstructure:"analysis_complete_fmt" validate:"required,contains=%d"` // Must contain %d for formatting
	NoProfiles              string `mapstructure:"no_profiles" validate:"required"`
	ProfilesHeader          string `mapstructure:"profiles_header" validate:"required"`
	EditUserUsage           string `mapstructure:"edit_user_usage" validate:"required"`
	EditUserInvalidID       string `mapstructure:"edit_user_invalid_id" validate:"required"`
	EditUserInvalidFieldFmt string `mapstructure:"edit_user_invalid_field_fmt" validate:"required,contains=%s"` // Must contain %s for formatting
	EditUserNotFoundFmt     string `mapstructure:"edit_user_not_found_fmt" validate:"required,contains=%d"`     // Must contain %d for formatting
	EditUserSuccessFmt      string `mapstructure:"edit_user_success_fmt" validate:"required,contains=%s"`       // Must contain %s for formatting
}

// LoadConfig reads configuration from file, environment variables, and validates it.
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	// Set default values
	// Logger defaults
	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.json", false)

	// Database defaults
	v.SetDefault("database.path", "storage.db")
	v.SetDefault("database.max_history_messages", 100)

	// Gemini API defaults
	v.SetDefault("gemini.model_name", "gemini-2.0-flash")
	v.SetDefault("gemini.temperature", 1.0)
	v.SetDefault("gemini.search_grounding", true)
	v.SetDefault("gemini.system_instruction", "You're a helpful assistant.")

	// Scheduler defaults - empty map as default
	v.SetDefault("scheduler.tasks", map[string]TaskConfig{
		"sql_maintenance": {
			Schedule: "0 3 * * *", // Daily at 3 AM
			Enabled:  true,
		},
		"profile_update": {
			Schedule: "0 1 * * *", // Daily at 1 AM
			Enabled:  true,
		},
	})

	// Default Messages
	v.SetDefault("messages.welcome", "Hello! Use @botname to chat with me.")
	v.SetDefault("messages.general_error", "Sorry, something went wrong. Please try again later.")
	v.SetDefault("messages.not_authorized", "You are not authorized to use this command.")
	v.SetDefault("messages.help", "Available commands:\n/help - Show this help message\nUse @botname to chat with the bot.\n\nAdmin commands:\n/mrl_reset - Delete all message history and profiles\n/mrl_analyze - Force analysis of unprocessed messages\n/mrl_profiles - Show all stored user profiles\n/mrl_edit_user <user_id> <field> <value> - Manually edit a user profile field (fields: aliases, origin_location, current_location, age_range, traits)")
	v.SetDefault("messages.history_reset", "All message history and user profiles have been soft-deleted.")
	v.SetDefault("messages.analyzing", "Analyzing unprocessed messages to update user profiles...")
	v.SetDefault("messages.analysis_no_messages", "No new messages found to analyze.")
	v.SetDefault("messages.analysis_complete_fmt", "Analysis complete. Processed %d messages. Updated/created %d profiles.")
	v.SetDefault("messages.no_profiles", "No user profiles found in the database.")
	v.SetDefault("messages.profiles_header", "--- User Profiles ---\nUserID | Aliases | Origin | Current | Age | Traits\n--------------------------------------------------\n")
	v.SetDefault("messages.edit_user_usage", "Usage: /mrl_edit_user <user_id> <field_name> <new_value...>")
	v.SetDefault("messages.edit_user_invalid_id", "Invalid User ID provided. It must be a number.")
	v.SetDefault("messages.edit_user_invalid_field_fmt", "Invalid field name: '%s'. Allowed fields are: %s")
	v.SetDefault("messages.edit_user_not_found_fmt", "User profile not found for ID: %d")
	v.SetDefault("messages.edit_user_success_fmt", "Successfully updated field '%s' for user %d.")

	// Set config file path, name, and type
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.AddConfigPath(".")
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok || configPath != "" {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// If config file not found and no specific path was given, defaults are used.
	}

	// Enable environment variable overriding
	v.SetEnvPrefix("BOT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Unmarshal the config into the Config struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// --- Validation ---
	validate := validator.New()

	if err := validate.Struct(&cfg); err != nil {
		// Improve error message to be more specific about validation failures
		validationErrors, ok := err.(validator.ValidationErrors)
		if !ok {
			return nil, fmt.Errorf("config validation failed: %w", err) // Should not happen
		}
		// Build a more informative error string
		var errorMsgs []string
		for _, e := range validationErrors {
			errorMsgs = append(errorMsgs, fmt.Sprintf("Field '%s': validation '%s' failed (value: '%v')", e.Namespace(), e.Tag(), e.Value()))
		}
		return nil, fmt.Errorf("config validation failed: %s", strings.Join(errorMsgs, "; "))
	}
	// --- End Validation ---

	return &cfg, nil
}
