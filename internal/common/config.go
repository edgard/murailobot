package common

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config defines the application configuration parameters.
// It includes settings for logging, bot operation, AI integration,
// and database configuration.
type Config struct {
	// Log level configuration
	LogLevel string `koanf:"log_level" validate:"oneof=debug info warn error"`

	// Bot configuration
	BotToken   string `koanf:"bot_token"    validate:"required"`
	BotAdminID int64  `koanf:"bot_admin_id" validate:"required,gt=0"`

	// Bot message templates (optional, defaults provided)
	BotMsgWelcome        string `koanf:"bot_msg_welcome"`
	BotMsgNotAuthorized  string `koanf:"bot_msg_not_authorized"`
	BotMsgProvideMessage string `koanf:"bot_msg_provide_message"`
	BotMsgGeneralError   string `koanf:"bot_msg_general_error"`
	BotMsgHistoryReset   string `koanf:"bot_msg_history_reset"`
	BotMsgAnalyzing      string `koanf:"bot_msg_analyzing"`
	BotMsgNoProfiles     string `koanf:"bot_msg_no_profiles"`
	BotMsgProfilesHeader string `koanf:"bot_msg_profiles_header"`

	// Bot command descriptions (optional, defaults provided)
	BotCmdStart    string `koanf:"bot_cmd_start"`
	BotCmdReset    string `koanf:"bot_cmd_reset"`
	BotCmdAnalyze  string `koanf:"bot_cmd_analyze"`
	BotCmdProfiles string `koanf:"bot_cmd_profiles"`
	BotCmdEditUser string `koanf:"bot_cmd_edit_user"`

	// AI service configuration
	AIToken              string        `koanf:"ai_token"               validate:"required"`
	AIBaseURL            string        `koanf:"ai_base_url"`
	AIModel              string        `koanf:"ai_model"               validate:"required"`
	AITemperature        float32       `koanf:"ai_temperature"         validate:"gte=0,lte=1"`
	AIInstruction        string        `koanf:"ai_instruction"         validate:"required"`
	AIProfileInstruction string        `koanf:"ai_profile_instruction" validate:"required"`
	AITimeout            time.Duration `koanf:"ai_timeout"             validate:"required,min=1s"`
	AIMaxContextTokens   int           `koanf:"ai_max_context_tokens"  validate:"min=1000"`

	// Database configuration
	DBPath string `koanf:"db_path" validate:"required"`
}

// LoadConfig loads configuration from config.yaml, sets default values,
// and validates the configuration. If the config file doesn't exist,
// it uses default values for optional fields.
func LoadConfig() (*Config, error) {
	config := &Config{}
	setDefaults(config)

	configPath := "config.yaml"
	k := koanf.New(".")

	if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := k.Unmarshal("", config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	// Log configuration summary
	slog.Info("configuration loaded",
		"log_level", config.LogLevel,
		"ai_model", config.AIModel,
		"ai_max_tokens", config.AIMaxContextTokens,
		"ai_temperature", config.AITemperature,
		"db_path", config.DBPath)

	return config, nil
}

// validateConfig performs validation of the configuration
func validateConfig(config *Config) error {
	if err := validator.New().Struct(config); err != nil {
		var validationErrors []string
		if errs, ok := err.(validator.ValidationErrors); ok {
			for _, e := range errs {
				validationErrors = append(validationErrors,
					fmt.Sprintf("%s: failed %s validation", e.Field(), e.Tag()))
			}
			return fmt.Errorf("configuration validation failed:\n- %s",
				strings.Join(validationErrors, "\n- "))
		}
		return fmt.Errorf("configuration validation failed: %w", err)
	}
	return nil
}

// setDefaults initializes configuration with sensible default values
func setDefaults(config *Config) {
	// Logging defaults
	config.LogLevel = "info"

	// AI service defaults
	config.AIBaseURL = "https://api.openai.com/v1"
	config.AIModel = "gpt-3.5-turbo" // Default to GPT-3.5 for cost efficiency
	config.AITemperature = 0.7       // Default temperature for balanced creativity
	config.AIMaxContextTokens = 4000 // Default context window size
	config.AITimeout = 2 * time.Minute

	// Database defaults
	config.DBPath = "storage.db"

	// Bot message template defaults
	config.BotMsgWelcome = "I'm ready to assist you. Mention me in your group message to start a conversation."
	config.BotMsgNotAuthorized = "You are not authorized to use this command."
	config.BotMsgProvideMessage = "Please provide a message."
	config.BotMsgGeneralError = "An error occurred. Please try again later."
	config.BotMsgHistoryReset = "History has been reset."
	config.BotMsgAnalyzing = "Analyzing messages..."
	config.BotMsgNoProfiles = "No user profiles found."
	config.BotMsgProfilesHeader = "User Profiles:\n\n"

	// Bot command description defaults
	config.BotCmdStart = "Start conversation with the bot"
	config.BotCmdReset = "Reset chat history (admin only)"
	config.BotCmdAnalyze = "Analyze messages and update profiles (admin only)"
	config.BotCmdProfiles = "Show user profiles (admin only)"
	config.BotCmdEditUser = "Edit user profile data (admin only)"

	// Default instructions
	config.AIInstruction = `You are a helpful and engaging group chat bot. ` +
		`Your responses should be concise, relevant, and natural. ` +
		`Feel free to use appropriate emoji occasionally. ` +
		`When users share personal information, remember it for future context.`

	config.AIProfileInstruction = `Analyze the user's messages to identify: ` +
		`- Display names and nicknames they use ` +
		`- Location information (current and origin) ` +
		`- Approximate age range based on context ` +
		`- Notable personality traits and interests ` +
		`Focus on clear patterns and explicitly stated information. ` +
		`Avoid speculative assumptions. ` +
		`If certain information is not available, leave those fields empty.`
}
