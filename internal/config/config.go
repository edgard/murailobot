// Package config provides configuration loading, validation, and management
// for the MurailoBot application. It handles reading from YAML files,
// setting default values, and validating configuration parameters.
package config

import (
	"log/slog"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config defines the application configuration parameters for all components
// of the MurailoBot system, including logging, bot settings, AI integration,
// and database configuration.
type Config struct {
	LogLevel string `koanf:"log_level" validate:"oneof=debug info warn error"`

	BotToken   string `koanf:"bot_token"    validate:"required"`
	BotAdminID int64  `koanf:"bot_admin_id" validate:"required,gt=0"`

	BotMsgWelcome        string `koanf:"bot_msg_welcome"`
	BotMsgNotAuthorized  string `koanf:"bot_msg_not_authorized"`
	BotMsgProvideMessage string `koanf:"bot_msg_provide_message"`
	BotMsgGeneralError   string `koanf:"bot_msg_general_error"`
	BotMsgHistoryReset   string `koanf:"bot_msg_history_reset"`
	BotMsgAnalyzing      string `koanf:"bot_msg_analyzing"`
	BotMsgNoProfiles     string `koanf:"bot_msg_no_profiles"`
	BotMsgProfilesHeader string `koanf:"bot_msg_profiles_header"`

	BotCmdStart    string `koanf:"bot_cmd_start"`
	BotCmdReset    string `koanf:"bot_cmd_reset"`
	BotCmdAnalyze  string `koanf:"bot_cmd_analyze"`
	BotCmdProfiles string `koanf:"bot_cmd_profiles"`
	BotCmdEditUser string `koanf:"bot_cmd_edit_user"`

	AIToken              string        `koanf:"ai_token"               validate:"required"`
	AIBaseURL            string        `koanf:"ai_base_url"            validate:"url"`
	AIModel              string        `koanf:"ai_model"`
	AITemperature        float32       `koanf:"ai_temperature"         validate:"min=0,max=2"`
	AIInstruction        string        `koanf:"ai_instruction"         validate:"required"`
	AIProfileInstruction string        `koanf:"ai_profile_instruction" validate:"required"`
	AITimeout            time.Duration `koanf:"ai_timeout"             validate:"min=1s,max=10m"`
	AIMaxContextTokens   int           `koanf:"ai_max_context_tokens"  validate:"min=1000,max=200000"`

	DBPath string `koanf:"db_path"`
}

// Load reads configuration from config.yaml, sets default values for
// optional fields, and validates the configuration. If the config file
// doesn't exist, it uses default values for all optional fields.
//
// Returns the validated configuration or an error if loading or validation fails.
func Load() (*Config, error) {
	startTime := time.Now()
	slog.Info("loading configuration")

	config := &Config{}

	// Set default values
	slog.Debug("setting default configuration values")
	setDefaults(config)

	// Load configuration from file
	configPath := "config.yaml"
	slog.Debug("attempting to load configuration file", "path", configPath)

	k := koanf.New(".")
	if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
		if !os.IsNotExist(err) {
			slog.Error("failed to load configuration file",
				"error", err,
				"path", configPath)
			return nil, err
		}
		slog.Info("configuration file not found, using defaults", "path", configPath)
	} else {
		slog.Debug("configuration file loaded successfully", "path", configPath)

		if err := k.Unmarshal("", config); err != nil {
			slog.Error("failed to parse configuration",
				"error", err,
				"path", configPath)
			return nil, err
		}
		slog.Debug("configuration unmarshalled successfully")
	}

	// Validate configuration
	slog.Debug("validating configuration")
	if err := validator.New().Struct(config); err != nil {
		slog.Error("configuration validation failed", "error", err)
		return nil, err
	}

	// Log key configuration values
	slog.Info("configuration loaded successfully",
		"log_level", config.LogLevel,
		"ai_model", config.AIModel,
		"db_path", config.DBPath,
		"duration_ms", time.Since(startTime).Milliseconds())

	// Log detailed configuration at debug level
	slog.Debug("detailed configuration",
		"ai_temperature", config.AITemperature,
		"ai_max_context_tokens", config.AIMaxContextTokens,
		"ai_timeout", config.AITimeout)

	return config, nil
}

func setDefaults(config *Config) {
	config.LogLevel = "info"

	config.AIBaseURL = "https://api.openai.com/v1"
	config.AIModel = "gpt-4o"
	config.AITemperature = 1.7
	config.AIMaxContextTokens = 16000
	config.AITimeout = 2 * time.Minute

	config.DBPath = "storage.db"

	config.BotMsgWelcome = "I'm ready to assist you. Mention me in your group message to start a conversation."
	config.BotMsgNotAuthorized = "You are not authorized to use this command."
	config.BotMsgProvideMessage = "Please provide a message."
	config.BotMsgGeneralError = "An error occurred. Please try again later."
	config.BotMsgHistoryReset = "History has been reset."
	config.BotMsgAnalyzing = "Analyzing messages..."
	config.BotMsgNoProfiles = "No user profiles found."
	config.BotMsgProfilesHeader = "User Profiles:\n\n"

	config.BotCmdStart = "Start conversation with the bot"
	config.BotCmdReset = "Reset chat history (admin only)"
	config.BotCmdAnalyze = "Analyze messages and update profiles (admin only)"
	config.BotCmdProfiles = "Show user profiles (admin only)"
	config.BotCmdEditUser = "Edit user profile data (admin only)"
}
