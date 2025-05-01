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
	// --- General Bot Messages ---
	StartWelcomeMsg string `mapstructure:"start_welcome_msg" validate:"required"`
	HelpMsg         string `mapstructure:"help_msg" validate:"required"`

	// --- Error & Fallback Messages ---
	ErrorGeneralMsg      string `mapstructure:"error_general_msg" validate:"required"`
	ErrorUnauthorizedMsg string `mapstructure:"error_unauthorized_msg" validate:"required"`

	// --- Mention Handler Specific ---
	MentionNoPromptMsg           string `mapstructure:"mention_no_prompt_msg" validate:"required"`
	MentionImageErrorMsg         string `mapstructure:"mention_image_error_msg" validate:"required"`
	MentionAIEmptyFallbackMsg    string `mapstructure:"mention_ai_empty_fallback_msg" validate:"required"`
	MentionEmptyReplyFallbackMsg string `mapstructure:"mention_empty_reply_fallback_msg" validate:"required"`

	// --- Admin Command: /mrl_reset ---
	ResetConfirmMsg string `mapstructure:"reset_confirm_msg" validate:"required"`
	ResetErrorMsg   string `mapstructure:"reset_error_msg" validate:"required"`
	ResetTimeoutMsg string `mapstructure:"reset_timeout_msg" validate:"required"`

	// --- Admin Command: /mrl_analyze ---
	AnalyzeProgressMsg   string `mapstructure:"analyze_progress_msg" validate:"required"`
	AnalyzeNoMessagesMsg string `mapstructure:"analyze_no_messages_msg" validate:"required"`
	AnalyzeCompleteFmt   string `mapstructure:"analyze_complete_fmt" validate:"required,contains=%d"` // Must contain %d for formatting
	AnalyzeTimeoutMsg    string `mapstructure:"analyze_timeout_msg" validate:"required"`

	// --- Admin Command: /mrl_profiles ---
	ProfilesEmptyMsg  string `mapstructure:"profiles_empty_msg" validate:"required"`
	ProfilesHeaderMsg string `mapstructure:"profiles_header_msg" validate:"required"`

	// --- Admin Command: /mrl_edit_user ---
	EditUserUsageMsg          string `mapstructure:"edit_user_usage_msg" validate:"required"`
	EditUserInvalidIDErrorMsg string `mapstructure:"edit_user_invalid_id_error_msg" validate:"required"`
	EditUserInvalidFieldFmt   string `mapstructure:"edit_user_invalid_field_fmt" validate:"required,contains=%s"` // Must contain %s for formatting
	EditUserNotFoundFmt       string `mapstructure:"edit_user_not_found_fmt" validate:"required,contains=%d"`     // Must contain %d for formatting
	EditUserSuccessFmt        string `mapstructure:"edit_user_success_fmt" validate:"required,contains=%s"`       // Must contain %s for formatting
	EditUserUpdateErrorFmt    string `mapstructure:"edit_user_update_error_fmt" validate:"required,contains=%s"`  // Must contain %s for formatting
	EditUserFetchErrorFmt     string `mapstructure:"edit_user_fetch_error_fmt" validate:"required,contains=%d"`   // Must contain %d for formatting
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

	// Default Messages (Grouped logically)
	// --- General Bot Messages ---
	v.SetDefault("messages.start_welcome_msg", "Hello! Mention me (@botname) or use /help to see what I can do.")
	v.SetDefault("messages.help_msg", "Here's how you can interact with me:\\n- Mention me (@botname) followed by your question or request.\\n- Use /help to see this message again.\\n\\nAdmin commands:\\n/mrl_reset - Delete all message history and profiles\\n/mrl_analyze - Force analysis of unprocessed messages\\n/mrl_profiles - Show all stored user profiles\\n/mrl_edit_user <user_id> <field> <value> - Manually edit a user profile field (fields: aliases, origin_location, current_location, age_range, traits)")

	// --- Error & Fallback Messages ---
	v.SetDefault("messages.error_general_msg", "Sorry, something went wrong. Please try again later.")
	v.SetDefault("messages.error_unauthorized_msg", "You are not authorized to use this command.")

	// --- Mention Handler Specific ---
	v.SetDefault("messages.mention_no_prompt_msg", "You mentioned me, but didn't provide a prompt. How can I help?")
	v.SetDefault("messages.mention_image_error_msg", "I couldn't process the image. Please try again.")
	v.SetDefault("messages.mention_ai_empty_fallback_msg", "I processed your message but couldn't generate a meaningful response. Could you rephrase or provide more context?")
	v.SetDefault("messages.mention_empty_reply_fallback_msg", "Sorry, I couldn't come up with a response for that.")

	// --- Admin Command: /mrl_reset ---
	v.SetDefault("messages.reset_confirm_msg", "All message history and user profiles have been deleted.")
	v.SetDefault("messages.reset_error_msg", "Sorry, I encountered an error while trying to reset the data.")
	v.SetDefault("messages.reset_timeout_msg", "The data reset operation timed out. Please try again later.")

	// --- Admin Command: /mrl_analyze ---
	v.SetDefault("messages.analyze_progress_msg", "Analyzing unprocessed messages to update user profiles...")
	v.SetDefault("messages.analyze_no_messages_msg", "No new messages found to analyze.")
	v.SetDefault("messages.analyze_complete_fmt", "Analysis complete. Processed %d messages. Updated/created %d profiles.")
	v.SetDefault("messages.analyze_timeout_msg", "The profile analysis operation timed out. Please try again later.")

	// --- Admin Command: /mrl_profiles ---
	v.SetDefault("messages.profiles_empty_msg", "No user profiles found in the database.")
	v.SetDefault("messages.profiles_header_msg", "UserID | Aliases | Origin | Current | Age | Traits\n\n")

	// --- Admin Command: /mrl_edit_user ---
	v.SetDefault("messages.edit_user_usage_msg", "Usage: /mrl_edit_user <user_id> <field_name> <new_value...>")
	v.SetDefault("messages.edit_user_invalid_id_error_msg", "Invalid User ID provided. It must be a number.")
	v.SetDefault("messages.edit_user_invalid_field_fmt", "Invalid field name: '%s'. Allowed fields are: %s")
	v.SetDefault("messages.edit_user_not_found_fmt", "User profile not found for ID: %d")
	v.SetDefault("messages.edit_user_success_fmt", "Successfully updated field '%s' for user %d.")
	v.SetDefault("messages.edit_user_update_error_fmt", "Sorry, I encountered an error while trying to update field '%s'.")
	v.SetDefault("messages.edit_user_fetch_error_fmt", "Sorry, I couldn't fetch the profile for user ID %d.")

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
