// Package config provides configuration handling for MurailoBot.
package config

import (
	"fmt"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"go.uber.org/zap"
)

// Default configuration file path
const DefaultConfigPath = "config.yaml"

// Config contains all the application configuration values.
type Config struct {
	// Bot configuration
	BotToken             string `koanf:"bot_token"        validate:"required"`
	BotAdminID           int64  `koanf:"bot_admin_id"     validate:"required"`
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

	// AI configuration
	AIToken              string        `koanf:"ai_token"               validate:"required"`
	AIBaseURL            string        `koanf:"ai_base_url"`
	AIModel              string        `koanf:"ai_model"`
	AITemperature        float32       `koanf:"ai_temperature"`
	AIMaxContextTokens   int           `koanf:"ai_max_context_tokens"`
	AITimeout            time.Duration `koanf:"ai_timeout"`
	AIInstruction        string        `koanf:"ai_instruction"`
	AIProfileInstruction string        `koanf:"ai_profile_instruction"`

	// Database configuration
	DBPath string `koanf:"db_path"`

	// Logging configuration
	LogFormat string `koanf:"log_format"`
	LogLevel  string `koanf:"log_level"`
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		// Bot default messages
		BotMsgWelcome:        "I'm ready to assist you. Mention me in your group message to start a conversation.",
		BotMsgNotAuthorized:  "You are not authorized to use this command.",
		BotMsgProvideMessage: "Please provide a message.",
		BotMsgGeneralError:   "An error occurred. Please try again later.",
		BotMsgHistoryReset:   "History has been reset.",
		BotMsgAnalyzing:      "Analyzing messages...",
		BotMsgNoProfiles:     "No user profiles found.",
		BotMsgProfilesHeader: "User Profiles:\n\n",

		// Bot default commands
		BotCmdStart:    "Start conversation with the bot",
		BotCmdReset:    "Reset chat history (admin only)",
		BotCmdAnalyze:  "Analyze messages and update profiles (admin only)",
		BotCmdProfiles: "Show user profiles (admin only)",
		BotCmdEditUser: "Edit user profile data (admin only)",

		// AI default values
		AIBaseURL:            "https://api.openai.com/v1",
		AIModel:              "gpt-4o",
		AITemperature:        1.7,
		AIMaxContextTokens:   16000,
		AITimeout:            2 * time.Minute,
		AIInstruction:        "You are a helpful assistant focused on providing clear and accurate responses.",
		AIProfileInstruction: "You are a behavioral analyst with expertise in psychology, linguistics, and social dynamics.\n\nYour task is to analyze chat messages and build concise, meaningful profiles of users.",

		// Database default values
		DBPath: "storage.db",

		// Logging default values
		LogFormat: "json",
		LogLevel:  "info",
	}
}

// LoadConfig reads configuration from the default YAML file and validates it.
func LoadConfig(logger *zap.Logger) (*Config, error) {
	// Start with default configuration
	cfg := DefaultConfig()

	k := koanf.New(".")

	// Load configuration from the default path
	if err := k.Load(file.Provider(DefaultConfigPath), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("failed to load config from %s: %w", DefaultConfigPath, err)
	}

	// Unmarshal to struct (this will override defaults with values from config file)
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	logger.Info("configuration loaded successfully", zap.String("config_file", DefaultConfigPath))

	return cfg, nil
}
