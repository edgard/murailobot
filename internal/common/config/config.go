// Package config provides configuration handling for MurailoBot.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

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

// Load reads configuration from a YAML file and validates it.
func Load(filePath string) (*Config, error) {
	k := koanf.New(".")

	// Load configuration
	if err := k.Load(file.Provider(filePath), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("failed to load config from %s: %w", filePath, err)
	}

	// Apply defaults for optional values
	applyDefaults(k)

	// Unmarshal to struct
	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Configure logging
	configureLogging(&cfg)

	slog.Info("configuration loaded successfully", "config_file", filePath)

	return &cfg, nil
}

func applyDefaults(k *koanf.Koanf) {
	// Bot default messages
	if !k.Exists("bot_msg_welcome") {
		k.Set("bot_msg_welcome", "Hello! I'm MurailoBot. Mention me in a group message to chat!")
	}

	if !k.Exists("bot_msg_not_authorized") {
		k.Set("bot_msg_not_authorized", "You are not authorized to use this command.")
	}

	if !k.Exists("bot_msg_provide_message") {
		k.Set("bot_msg_provide_message", "Please provide a message.")
	}

	if !k.Exists("bot_msg_general_error") {
		k.Set("bot_msg_general_error", "An error occurred. Please try again later.")
	}

	if !k.Exists("bot_msg_history_reset") {
		k.Set("bot_msg_history_reset", "Message history and user profiles have been reset.")
	}

	if !k.Exists("bot_msg_analyzing") {
		k.Set("bot_msg_analyzing", "Analyzing messages and updating user profiles...")
	}

	if !k.Exists("bot_msg_no_profiles") {
		k.Set("bot_msg_no_profiles", "No user profiles found. Try analyzing more messages first.")
	}

	if !k.Exists("bot_msg_profiles_header") {
		k.Set("bot_msg_profiles_header", "📊 User Profiles:\n\n")
	}

	// Bot command descriptions
	if !k.Exists("bot_cmd_start") {
		k.Set("bot_cmd_start", "Start the bot and get a welcome message")
	}

	if !k.Exists("bot_cmd_reset") {
		k.Set("bot_cmd_reset", "Reset message history and profiles")
	}

	if !k.Exists("bot_cmd_analyze") {
		k.Set("bot_cmd_analyze", "Analyze messages and update profiles")
	}

	if !k.Exists("bot_cmd_profiles") {
		k.Set("bot_cmd_profiles", "Show user profiles")
	}

	if !k.Exists("bot_cmd_edit_user") {
		k.Set("bot_cmd_edit_user", "Edit a user profile field")
	}

	// AI configuration defaults
	if !k.Exists("ai_base_url") {
		k.Set("ai_base_url", "https://api.openai.com/v1")
	}

	if !k.Exists("ai_model") {
		k.Set("ai_model", "gpt-3.5-turbo")
	}

	if !k.Exists("ai_temperature") {
		k.Set("ai_temperature", 0.8)
	}

	if !k.Exists("ai_timeout") {
		k.Set("ai_timeout", 30*time.Second)
	}

	if !k.Exists("ai_max_context_tokens") {
		k.Set("ai_max_context_tokens", 4000)
	}

	// Database configuration defaults
	if !k.Exists("db_path") {
		k.Set("db_path", "storage.db")
	}

	// Logging configuration defaults
	if !k.Exists("log_format") {
		k.Set("log_format", "json")
	}

	if !k.Exists("log_level") {
		k.Set("log_level", "info")
	}

	// Default system prompt if not provided
	if !k.Exists("ai_instruction") {
		k.Set("ai_instruction", `You are a helpful and friendly assistant in a Telegram group chat.
Your responses should be concise, helpful, and conversational.

When asked about technical topics, provide accurate information with examples where appropriate.
For subjective questions, present multiple perspectives and avoid strong bias.
Keep responses concise and to the point, use markdown formatting sparingly for readability.
Be friendly and casual in tone while remaining respectful.`)
	}

	// Default profile generation prompt if not provided
	if !k.Exists("ai_profile_instruction") {
		k.Set("ai_profile_instruction", `You are a behavioral analyst with expertise in psychology, linguistics, and social dynamics.

Your task is to analyze chat messages and build concise, meaningful profiles of users. Pay special attention to:

1. Names and nicknames they go by (displayNames)
2. Mentions of where they're from (originLocation)
3. Where they currently live (currentLocation)
4. Approximate age range based on references or communication style (ageRange)
5. Personality traits, interests, and characteristics (traits)

Focus on what the text directly reveals. Make cautious, conservative inferences where evidence is limited.
If information is missing, leave those fields empty rather than guessing.`)
	}
}

func configureLogging(cfg *Config) {
	var handler slog.Handler

	switch cfg.LogFormat {
	case "text":
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: getLogLevel(cfg.LogLevel),
		})
	default: // json
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: getLogLevel(cfg.LogLevel),
		})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}

func getLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
