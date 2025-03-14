// Package config manages application configuration from environment variables,
// config files, and default values.
package config

import (
	"time"
)

// Configuration constants.
const (
	// DefaultAITimeout defines the default timeout for AI API requests.
	DefaultAITimeout = 2 * time.Minute

	// DefaultLogLevel defines the default logging level.
	DefaultLogLevel = "info"

	// DefaultLogFormat defines the default logging format.
	DefaultLogFormat = "json"
)

// Default messages for Telegram bot responses.
const (
	// Default command descriptions for Telegram commands.
	DefaultStartCommandDescription    = "Start conversation with the bot"
	DefaultResetCommandDescription    = "Reset chat history (admin only)"
	DefaultAnalyzeCommandDescription  = "Analyze messages and update profiles (admin only)"
	DefaultProfilesCommandDescription = "Show user profiles (admin only)"
	DefaultEditUserCommandDescription = "Edit user profile data (admin only)"

	// Default messages for various bot responses.
	DefaultWelcomeMessage        = "👋 Welcome! I'm ready to assist you. Mention me in your group message to start a conversation."
	DefaultNotAuthorizedMessage  = "🚫 Access denied. Please contact the administrator."
	DefaultProvideMessagePrompt  = "ℹ️ Please provide a message when mentioning me."
	DefaultGeneralErrorMessage   = "❌ Error occurred. Please try again later."
	DefaultHistoryResetMessage   = "✅ Chat history has been cleared successfully."
	DefaultAnalyzingMessage      = "⏳ Analyzing messages and updating user profiles..."
	DefaultProfilesHeaderMessage = "👥 User Profiles\n\n"
	DefaultNoProfilesMessage     = "ℹ️ No user profiles available. Run /mrl_analyze to generate profiles."
	DefaultInvalidUserIDMessage  = "❌ Invalid user ID. Please provide a valid numeric ID."
	DefaultInvalidFieldMessage   = "❌ Invalid field. Please use: displaynames, origin, location, age, or traits."
	DefaultUpdateSuccessMessage  = "✅ Successfully updated %s for user %d to: %s"
	DefaultUserEditUsageMessage  = "ℹ️ Usage: /mrl_edit_user [user_id] [field] [new_value]\n\n" +
		"Fields:\n" +
		"- displaynames: User's display names\n" +
		"- origin: Origin location\n" +
		"- location: Current location\n" +
		"- age: Age range\n" +
		"- traits: Personality traits\n\n" +
		"Example: /mrl_edit_user 123456789 traits friendly, helpful, technical"
)

// defaultConfig holds the default configuration values.
var defaultConfig = map[string]any{
	"ai.base_url":    "https://api.openai.com/v1",
	"ai.model":       "gpt-4o",
	"ai.temperature": 1.7,
	"ai.instruction": "You are a helpful assistant focused on providing clear and accurate responses.",
	"ai.profile_instruction": `You are a behavioral analyst with expertise in psychology, linguistics, and social dynamics.
Your task is to analyze chat messages and build detailed psychological profiles of users.

## ANALYSIS APPROACH
When analyzing messages, pay attention to:
1. Language patterns, word choice, and communication style
2. Emotional expressions and reactions to different topics
3. Recurring themes or topics in their communications
4. How communication evolves over time (changing interests, language formality)
5. Interaction patterns with other users and group dynamics
6. Cultural references, language usage, and personal details they reveal
7. Privacy considerations - avoid including sensitive personal information

## PROFILE DATA GUIDELINES [CRITICAL]

### Preserving Important Information
- When existing profile information is provided, you MUST preserve all meaningful information
- Only replace existing profile fields when you have clear and specific new evidence
- For fields where you have no new information, keep the existing value
- If uncertain about any field, retain the existing information or use qualifiers like "possibly" or "appears to be"
- NEVER include sensitive personal information (addresses, phone numbers, financial details)

Example: If an existing profile has "origin_location": "Germany" but the messages don't mention location,
keep this value. Only update if there is clear evidence of a different origin location.

### Trait Quality and Consolidation
- You MUST consolidate similar traits while preserving their full meaning and nuance
- Even for existing profiles, consolidate redundant traits (this is the ONLY modification allowed without new evidence)
- Preserve distinctions between interests, expertise, personality traits, and preferences when consolidating
- Examples of proper consolidation:
  - "interested in gaming news" + "follows gaming trends" → "follows gaming trends and news"
  - "enjoys hiking" + "loves outdoor activities" → "enjoys outdoor activities, especially hiking"
  - "works in software development" + "programmer" → "software developer/programmer"
- Keep traits distinct and non-redundant within each user profile
- Use precise, concise language without repetition
- Focus on breadth of characteristics rather than variations of the same trait
- The final traits list should have NO redundant or overlapping traits
`,
	"ai.timeout": DefaultAITimeout,

	"telegram.commands.start":     DefaultStartCommandDescription,
	"telegram.commands.reset":     DefaultResetCommandDescription,
	"telegram.commands.analyze":   DefaultAnalyzeCommandDescription,
	"telegram.commands.profiles":  DefaultProfilesCommandDescription,
	"telegram.commands.edit_user": DefaultEditUserCommandDescription,

	"telegram.messages.welcome":         DefaultWelcomeMessage,
	"telegram.messages.not_authorized":  DefaultNotAuthorizedMessage,
	"telegram.messages.provide_message": DefaultProvideMessagePrompt,
	"telegram.messages.general_error":   DefaultGeneralErrorMessage,
	"telegram.messages.history_reset":   DefaultHistoryResetMessage,
	"telegram.messages.analyzing":       DefaultAnalyzingMessage,
	"telegram.messages.no_profiles":     DefaultNoProfilesMessage,
	"telegram.messages.invalid_user_id": DefaultInvalidUserIDMessage,
	"telegram.messages.invalid_field":   DefaultInvalidFieldMessage,
	"telegram.messages.update_success":  DefaultUpdateSuccessMessage,
	"telegram.messages.user_edit_usage": DefaultUserEditUsageMessage,
	"telegram.messages.profiles_header": DefaultProfilesHeaderMessage,

	"log.level":  DefaultLogLevel,
	"log.format": DefaultLogFormat,
}

// Config defines the application configuration.
// Values can be set via environment variables prefixed with BOT_ (e.g., BOT_AI_TOKEN)
// or through config.yaml.
type Config struct {
	// AI API Configuration
	//
	// AIToken is the authentication token for the OpenAI API
	AIToken string `koanf:"ai.token" validate:"required"`

	// AIBaseURL is the base URL for the OpenAI API (including version path)
	AIBaseURL string `koanf:"ai.base_url" validate:"required,url"`

	// AIModel specifies which GPT model to use (e.g., "gpt-4")
	AIModel string `koanf:"ai.model" validate:"required"`

	// AITemperature controls response randomness (0.0-2.0, higher = more random)
	AITemperature float32 `koanf:"ai.temperature" validate:"required,min=0,max=2"`

	// AIInstruction provides the system message for the AI
	AIInstruction string `koanf:"ai.instruction" validate:"required"`

	// AIProfileInstruction provides the system message for user profile generation
	AIProfileInstruction string `koanf:"ai.profile_instruction" validate:"required"`

	// AITimeout sets maximum duration for API requests
	AITimeout time.Duration `koanf:"ai.timeout" validate:"required,min=1s,max=10m"`

	// Telegram Bot Configuration
	//
	// TelegramToken is the bot's API authentication token
	TelegramToken string `koanf:"telegram.token" validate:"required"`

	// TelegramAdminID is the Telegram user ID of the bot administrator
	TelegramAdminID int64 `koanf:"telegram.admin_id" validate:"required,gt=0"`

	// Command Descriptions
	//
	// These define the descriptions shown for bot commands in Telegram
	TelegramStartCommandDescription    string `koanf:"telegram.commands.start"     validate:"required"`
	TelegramResetCommandDescription    string `koanf:"telegram.commands.reset"     validate:"required"`
	TelegramAnalyzeCommandDescription  string `koanf:"telegram.commands.analyze"   validate:"required"`
	TelegramProfilesCommandDescription string `koanf:"telegram.commands.profiles"  validate:"required"`
	TelegramEditUserCommandDescription string `koanf:"telegram.commands.edit_user" validate:"required"`

	// Message Templates
	//
	// These define the bot's response messages for different situations
	TelegramWelcomeMessage        string `koanf:"telegram.messages.welcome"         validate:"required"`
	TelegramNotAuthorizedMessage  string `koanf:"telegram.messages.not_authorized"  validate:"required"`
	TelegramProvideMessage        string `koanf:"telegram.messages.provide_message" validate:"required"`
	TelegramGeneralErrorMessage   string `koanf:"telegram.messages.general_error"   validate:"required"`
	TelegramHistoryResetMessage   string `koanf:"telegram.messages.history_reset"   validate:"required"`
	TelegramAnalyzingMessage      string `koanf:"telegram.messages.analyzing"       validate:"required"`
	TelegramNoProfilesMessage     string `koanf:"telegram.messages.no_profiles"     validate:"required"`
	TelegramInvalidUserIDMessage  string `koanf:"telegram.messages.invalid_user_id" validate:"required"`
	TelegramInvalidFieldMessage   string `koanf:"telegram.messages.invalid_field"   validate:"required"`
	TelegramUpdateSuccessMessage  string `koanf:"telegram.messages.update_success"  validate:"required"`
	TelegramUserEditUsageMessage  string `koanf:"telegram.messages.user_edit_usage" validate:"required"`
	TelegramProfilesHeaderMessage string `koanf:"telegram.messages.profiles_header" validate:"required"`

	// Logging Configuration
	//
	// LogLevel sets the minimum logging level (debug|info|warn|error)
	LogLevel string `koanf:"log.level" validate:"required,oneof=debug info warn error"`

	// LogFormat specifies the log output format (json|text)
	LogFormat string `koanf:"log.format" validate:"required,oneof=json text"`
}
