package common

import "errors"

var (
	// Input validation errors
	ErrInvalidInput    = errors.New("invalid input")
	ErrEmptyInput      = errors.New("empty input")
	ErrNilInput        = errors.New("nil input")
	ErrInvalidUserID   = errors.New("invalid user ID")
	ErrInvalidLimit    = errors.New("invalid limit")
	ErrInvalidTimeZone = errors.New("invalid timezone")
	ErrInvalidMessage  = errors.New("invalid message format")
	ErrInvalidJSON     = errors.New("invalid JSON format")
	ErrInvalidLogLevel = errors.New("invalid log level")

	// Configuration errors
	ErrInvalidConfig = errors.New("invalid configuration")
	ErrMissingToken  = errors.New("required token is missing")
	ErrBotRequired   = errors.New("bot interface is required")

	// Client errors
	ErrNilRequest       = errors.New("nil request")
	ErrNilJobFunction   = errors.New("nil job function")
	ErrNilTelegramAPI   = errors.New("nil telegram API")
	ErrNilOpenAIClient  = errors.New("nil OpenAI client")
	ErrNilStorageClient = errors.New("nil storage client")

	// Authorization errors
	ErrUnauthorized    = errors.New("unauthorized access")
	ErrAPITokenRevoked = errors.New("API token revoked")

	// API errors
	ErrNoResponse        = errors.New("no response from service")
	ErrNoResponseChoices = errors.New("no response choices available")
	ErrTokenCount        = errors.New("failed to count tokens")
	ErrAPITimeout        = errors.New("API request timed out")

	// Database errors
	ErrDatabaseConnection = errors.New("database connection lost")
	ErrDatabaseInit       = errors.New("failed to initialize database")
	ErrDatabaseOp         = errors.New("database operation failed")
	ErrDatabaseMigration  = errors.New("database migration failed")

	// Message processing errors
	ErrNoMessagesToAnalyze  = errors.New("no messages to analyze")
	ErrEmptyProfileResponse = errors.New("empty profile response")
	ErrMessageSave          = errors.New("failed to save message")
	ErrMessageFetch         = errors.New("failed to fetch messages")
	ErrMessageDelete        = errors.New("failed to delete messages")

	// Profile errors
	ErrProfileSave   = errors.New("failed to save profile")
	ErrProfileFetch  = errors.New("failed to fetch profile")
	ErrProfileUpdate = errors.New("failed to update profile")

	// Job scheduling errors
	ErrEmptyJobName        = errors.New("empty job name")
	ErrEmptyCronExpression = errors.New("empty cron expression")
	ErrDuplicateJob        = errors.New("job already exists")
	ErrJobSchedule         = errors.New("failed to schedule job")

	// Service lifecycle errors
	ErrInitialization  = errors.New("initialization failed")
	ErrServiceStart    = errors.New("failed to start service")
	ErrServiceStop     = errors.New("failed to stop service")
	ErrShutdownTimeout = errors.New("shutdown timed out")
)
