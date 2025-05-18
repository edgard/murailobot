// Package config handles configuration loading and validation for MurailoBot.
// It uses Viper for configuration management and go-playground/validator for validation.
package config

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/go-telegram/bot/models"
	"github.com/spf13/viper"
)

// Config is the main configuration structure that holds all settings for the application.
// It contains nested configuration structures for different components.
type Config struct {
	Telegram  TelegramConfig    `mapstructure:"telegram" validate:"required"`
	Database  DatabaseConfig    `mapstructure:"database" validate:"required"`
	Gemini    GeminiConfig      `mapstructure:"gemini" validate:"required"`
	Logger    LoggerConfig      `mapstructure:"logger" validate:"required"`
	Scheduler SchedulerConfig   `mapstructure:"scheduler" validate:"required"`
	Messages  BotMessagesConfig `mapstructure:"messages" validate:"required"`
}

// TelegramConfig holds configuration for the Telegram bot API connection.
type TelegramConfig struct {
	Token       string       `mapstructure:"token" validate:"required,min=45,max=50"`
	AdminUserID int64        `mapstructure:"admin_user_id" validate:"required,gt=0"`
	BotInfo     *models.User `mapstructure:"-" validate:"-"`
}

// DatabaseConfig holds configuration for the database connection.
type DatabaseConfig struct {
	Path               string `mapstructure:"path" validate:"required,filepath"`
	MaxHistoryMessages int    `mapstructure:"max_history_messages" validate:"required,gt=0"`
}

// GeminiConfig holds configuration for the Google Gemini AI API.
type GeminiConfig struct {
	APIKey            string  `mapstructure:"api_key" validate:"required,min=10"`
	ModelName         string  `mapstructure:"model_name" validate:"required"`
	Temperature       float32 `mapstructure:"temperature" validate:"required,gte=0,lte=2"`
	SystemInstruction string  `mapstructure:"system_instruction" validate:"required"`
	MaxRetries        int     `mapstructure:"max_retries" validate:"gte=0"`
	RetryDelaySeconds int     `mapstructure:"retry_delay_seconds" validate:"gte=0"`
}

// LoggerConfig holds configuration for the logger component.
type LoggerConfig struct {
	Level string `mapstructure:"level" validate:"required,oneof=debug info warn error"`
	JSON  bool   `mapstructure:"json"`
}

// SchedulerConfig holds configuration for the task scheduler.
type SchedulerConfig struct {
	Tasks map[string]TaskConfig `mapstructure:"tasks" validate:"required,dive"`
}

// TaskConfig holds configuration for an individual scheduled task.
type TaskConfig struct {
	Schedule string `mapstructure:"schedule" validate:"required,cron"`
	Enabled  bool   `mapstructure:"enabled"`
}

// BotMessagesConfig holds all message templates used by the bot.
type BotMessagesConfig struct {
	StartWelcomeMsg string `mapstructure:"start_welcome_msg" validate:"required"`
	HelpMsg         string `mapstructure:"help_msg" validate:"required"`

	ErrorGeneralMsg      string `mapstructure:"error_general_msg" validate:"required"`
	ErrorUnauthorizedMsg string `mapstructure:"error_unauthorized_msg" validate:"required"`

	MentionNoPromptMsg           string `mapstructure:"mention_no_prompt_msg" validate:"required"`
	MentionImageErrorMsg         string `mapstructure:"mention_image_error_msg" validate:"required"`
	MentionAIEmptyFallbackMsg    string `mapstructure:"mention_ai_empty_fallback_msg" validate:"required"`
	MentionEmptyReplyFallbackMsg string `mapstructure:"mention_empty_reply_fallback_msg" validate:"required"`

	ResetConfirmMsg string `mapstructure:"reset_confirm_msg" validate:"required"`
	ResetErrorMsg   string `mapstructure:"reset_error_msg" validate:"required"`
	ResetTimeoutMsg string `mapstructure:"reset_timeout_msg" validate:"required"`

	AnalyzeProgressMsg   string `mapstructure:"analyze_progress_msg" validate:"required"`
	AnalyzeNoMessagesMsg string `mapstructure:"analyze_no_messages_msg" validate:"required"`
	AnalyzeCompleteFmt   string `mapstructure:"analyze_complete_fmt" validate:"required,contains=%d"`
	AnalyzeTimeoutMsg    string `mapstructure:"analyze_timeout_msg" validate:"required"`

	ProfilesEmptyMsg  string `mapstructure:"profiles_empty_msg" validate:"required"`
	ProfilesHeaderMsg string `mapstructure:"profiles_header_msg" validate:"required"`

	EditUserUsageMsg          string `mapstructure:"edit_user_usage_msg" validate:"required"`
	EditUserInvalidIDErrorMsg string `mapstructure:"edit_user_invalid_id_error_msg" validate:"required"`
	EditUserInvalidFieldFmt   string `mapstructure:"edit_user_invalid_field_fmt" validate:"required,contains=%s"`
	EditUserNotFoundFmt       string `mapstructure:"edit_user_not_found_fmt" validate:"required,contains=%d"`
	EditUserSuccessFmt        string `mapstructure:"edit_user_success_fmt" validate:"required,contains=%s"`
	EditUserUpdateErrorFmt    string `mapstructure:"edit_user_update_error_fmt" validate:"required,contains=%s"`
	EditUserFetchErrorFmt     string `mapstructure:"edit_user_fetch_error_fmt" validate:"required,contains=%d"`
}

// LoadConfig loads the configuration from the specified path and validates it.
// It returns the validated configuration or an error if loading or validation fails.
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.json", false)

	v.SetDefault("database.path", "storage.db")
	v.SetDefault("database.max_history_messages", 100)

	v.SetDefault("gemini.model_name", "gemini-2.0-flash")
	v.SetDefault("gemini.temperature", 1.0)
	v.SetDefault("gemini.system_instruction", "You are MurailoBot, a helpful AI assistant integrated into a Telegram group chat. Your primary goal is to understand the users based on their messages and maintain profiles about them. You can also answer general questions. Be concise and helpful.")
	v.SetDefault("gemini.max_retries", 3)
	v.SetDefault("gemini.retry_delay_seconds", 2)

	v.SetDefault("scheduler.tasks", map[string]TaskConfig{
		"sql_maintenance": {
			Schedule: "0 3 * * *",
			Enabled:  true,
		},
	})

	v.SetDefault("messages.start_welcome_msg", "Hello!. Mention me or use /help to see what I can do.")
	v.SetDefault("messages.help_msg", "Here's how you can interact with me:\n- Mention me followed by your question or request.\n- Reply directly to one of my messages.\n- Use /help to see this message again.\n\nAdmin commands:\n/mrl_reset - Delete all message history and profiles\n/mrl_analyze - Force analysis of unprocessed messages\n/mrl_profiles - Show all stored user profiles\n/mrl_edit_user <user_id> <field> <value> - Manually edit a user profile field (fields: aliases, origin_location, current_location, age_range, traits)")

	v.SetDefault("messages.error_general_msg", "Sorry, something went wrong on my end. Please try again later or contact the admin.")
	v.SetDefault("messages.error_unauthorized_msg", "⛔ You are not authorized to use this command.")

	v.SetDefault("messages.mention_no_prompt_msg", "You mentioned me, but didn't provide a prompt. How can I help?")
	v.SetDefault("messages.mention_image_error_msg", "Sorry, I couldn't process the image you sent. Please try again.")
	v.SetDefault("messages.mention_ai_empty_fallback_msg", "I processed your request but couldn't generate a meaningful response. Could you try rephrasing or providing more context?")
	v.SetDefault("messages.mention_empty_reply_fallback_msg", "Sorry, I couldn't come up with a response for that.")

	v.SetDefault("messages.reset_confirm_msg", "✅ All message history and user profiles have been successfully deleted.")
	v.SetDefault("messages.reset_error_msg", "❌ Error: Failed to reset data. Please check the logs.")
	v.SetDefault("messages.reset_timeout_msg", "⏳ Warning: The data reset operation timed out. It might be partially complete. Please check the logs.")

	v.SetDefault("messages.analyze_progress_msg", "⏳ Analyzing unprocessed messages to update user profiles...")
	v.SetDefault("messages.analyze_no_messages_msg", "ℹ️ No new messages found to analyze.")
	v.SetDefault("messages.analyze_complete_fmt", "✅ Analysis complete. Processed %d messages. Updated/created %d profiles.")
	v.SetDefault("messages.analyze_timeout_msg", "⏳ Warning: The profile analysis operation timed out. It might be partially complete. Please check the logs.")

	v.SetDefault("messages.profiles_empty_msg", "ℹ️ No user profiles found in the database.")
	v.SetDefault("messages.profiles_header_msg", "👤 **Stored User Profiles** 👤\n\nUserID | Aliases | Origin | Current | Age | Traits\n--------------------------------------------------\n")

	v.SetDefault("messages.edit_user_usage_msg", "⚠️ Usage: /mrl_edit_user <user_id> <field_name> <new_value...>\nExample: /mrl_edit_user 12345 traits friendly, helpful")
	v.SetDefault("messages.edit_user_invalid_id_error_msg", "❌ Error: Invalid User ID provided. It must be a number.")
	v.SetDefault("messages.edit_user_invalid_field_fmt", "❌ Error: Invalid field name: '%s'. Allowed fields are: %s")
	v.SetDefault("messages.edit_user_not_found_fmt", "❌ Error: User profile not found for ID: %d")
	v.SetDefault("messages.edit_user_success_fmt", "✅ Successfully updated field '%s' for user %d.")
	v.SetDefault("messages.edit_user_update_error_fmt", "❌ Error: Failed to update field '%s'. Please check the logs.")
	v.SetDefault("messages.edit_user_fetch_error_fmt", "❌ Error: Could not fetch the profile for user ID %d. Please check the logs.")

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.AddConfigPath(".")
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok || configPath != "" {
			return nil, fmt.Errorf("failed to read config file '%s': %w", v.ConfigFileUsed(), err)
		}

		fmt.Printf("INFO: Config file not found at default location, using defaults and environment variables.\n")
	} else {
		fmt.Printf("INFO: Using configuration file: %s\n", v.ConfigFileUsed())
	}

	v.SetEnvPrefix("BOT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	validate := validator.New()
	if err := validate.Struct(&cfg); err != nil {
		validationErrors, ok := err.(validator.ValidationErrors)
		if !ok {
			return nil, fmt.Errorf("config validation failed with unexpected error type: %w", err)
		}

		var errorMsgs []string
		for _, e := range validationErrors {
			errorMsgs = append(errorMsgs, fmt.Sprintf("Field '%s': validation '%s' failed (value: '%v')", e.Namespace(), e.Tag(), e.Value()))
		}
		return nil, fmt.Errorf("configuration validation failed: %s", strings.Join(errorMsgs, "; "))
	}

	return &cfg, nil
}
