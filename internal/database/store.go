package database

import (
	"context"
	"database/sql" // Needed for sql.ErrNoRows and potentially *sql.NullString
	"errors"       // Needed for errors.Is
	"fmt"
	"io"
	"log/slog"
	"time" // Needed for UpdatedAt

	"github.com/jmoiron/sqlx"
)

// Store defines the interface for database operations.
// Methods should accept context.Context for cancellation and timeouts.
type Store interface {
	// Ping checks the database connection.
	Ping(ctx context.Context) error

	// SaveMessage inserts a new message record.
	SaveMessage(ctx context.Context, message *Message) error

	// GetRecentMessagesInChat retrieves the most recent 'limit' messages for a given chat ID.
	GetRecentMessagesInChat(ctx context.Context, chatID int64, limit int) ([]Message, error)

	// RunSQLMaintenance performs database maintenance tasks like VACUUM.
	RunSQLMaintenance(ctx context.Context) error

	// --- Methods to be added for ported features ---

	// GetUserProfile retrieves a user profile by user ID. Returns nil, nil if not found.
	GetUserProfile(ctx context.Context, userID int64) (*UserProfile, error)

	// GetAllUserProfiles retrieves all user profiles.
	GetAllUserProfiles(ctx context.Context) (map[int64]*UserProfile, error)

	// SaveUserProfile inserts or updates a user profile.
	SaveUserProfile(ctx context.Context, profile *UserProfile) error

	// GetUnprocessedMessages retrieves messages that haven't been processed for profile analysis.
	GetUnprocessedMessages(ctx context.Context) ([]*Message, error)

	// MarkMessagesAsProcessed marks a list of messages as processed.
	MarkMessagesAsProcessed(ctx context.Context, messageIDs []uint) error

	// DeleteAllMessages deletes all messages (used by reset command).
	DeleteAllMessages(ctx context.Context) error

	// DeleteAllUserProfiles deletes all user profiles (used by reset command).
	DeleteAllUserProfiles(ctx context.Context) error

	// GetRecentMessages retrieves recent messages across all chats, with pagination.
	GetRecentMessages(ctx context.Context, chatID int64, limit int, beforeID uint) ([]*Message, error)

	// DeleteAllMessagesAndProfiles deletes all messages and user profiles in a single transaction.
	// This ensures that either all data is deleted or none is (atomicity).
	DeleteAllMessagesAndProfiles(ctx context.Context) error
}

// sqlxStore provides an implementation of the Store interface using sqlx.
type sqlxStore struct {
	db     *sqlx.DB
	logger *slog.Logger
}

// NewStore creates a new Store implementation backed by sqlx.
// It requires a connected sqlx.DB instance and a logger.
func NewStore(db *sqlx.DB, logger *slog.Logger) Store {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &sqlxStore{
		db:     db,
		logger: logger.With("component", "store"),
	}
}

// Ping checks the database connection.
func (s *sqlxStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// SaveMessage inserts a new message record.
// Now includes parameter validation and transaction support.
func (s *sqlxStore) SaveMessage(ctx context.Context, message *Message) error {
	// Validate input parameters
	if message == nil {
		return fmt.Errorf("cannot save nil message")
	}

	// Basic validation of required fields
	if message.ChatID == 0 {
		return fmt.Errorf("message must have a non-zero chat_id")
	}
	if message.UserID == 0 {
		return fmt.Errorf("message must have a non-zero user_id")
	}
	if message.Content == "" {
		return fmt.Errorf("message must have non-empty content")
	}
	if message.Timestamp.IsZero() {
		return fmt.Errorf("message must have a non-zero timestamp")
	}

	// Ensure CreatedAt and UpdatedAt are set
	now := time.Now().UTC()
	message.CreatedAt = now
	message.UpdatedAt = now

	// Start a transaction
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to begin transaction for saving message",
			"chat_id", message.ChatID, "user_id", message.UserID, "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				if !errors.Is(rollbackErr, sql.ErrTxDone) {
					s.logger.WarnContext(ctx, "Error rolling back transaction", "error", rollbackErr)
				}
			}
		}
	}()

	query := `
        INSERT INTO messages (chat_id, user_id, content, timestamp, created_at, updated_at, processed_at)
        VALUES (:chat_id, :user_id, :content, :timestamp, :created_at, :updated_at, :processed_at);
    `

	result, err := tx.NamedExecContext(ctx, query, message)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error saving message", "chat_id", message.ChatID, "user_id", message.UserID, "error", err)
		return fmt.Errorf("failed to save message (chat %d, user %d): %w", message.ChatID, message.UserID, err)
	}

	// Get the last inserted ID
	id, err := result.LastInsertId()
	if err == nil {
		//nolint:gosec // integer overflow conversion is acceptable here
		message.ID = uint(id) // Update the message struct with the generated ID
	} else {
		// Log if getting LastInsertId fails, but don't fail the operation
		s.logger.WarnContext(ctx, "Could not retrieve last insert ID after saving message",
			"chat_id", message.ChatID, "user_id", message.UserID, "error", err)
	}

	// Check that we affected exactly one row
	affected, err := result.RowsAffected()
	if err == nil && affected != 1 {
		s.logger.WarnContext(ctx, "Unexpected number of rows affected when saving message",
			"chat_id", message.ChatID, "user_id", message.UserID, "affected", affected)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		s.logger.ErrorContext(ctx, "Failed to commit transaction",
			"chat_id", message.ChatID, "user_id", message.UserID, "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	// Successfully committed, set tx to nil to avoid rollback
	tx = nil

	s.logger.DebugContext(ctx, "Message saved successfully",
		"chat_id", message.ChatID, "user_id", message.UserID, "message_id", message.ID)
	return nil
}

// GetRecentMessagesInChat retrieves the most recent 'limit' messages for a given chat ID.
// Now includes parameter validation and context timeout checks.
func (s *sqlxStore) GetRecentMessagesInChat(ctx context.Context, chatID int64, limit int) ([]Message, error) {
	// Parameter validation
	if chatID == 0 {
		return nil, fmt.Errorf("chat_id cannot be zero")
	}

	// Check for reasonable limits
	if limit <= 0 {
		limit = 20 // Default to reasonable limit if none provided or invalid
		s.logger.DebugContext(ctx, "Invalid limit provided, using default", "chat_id", chatID, "default_limit", limit)
	} else if limit > 100 {
		limit = 100 // Cap maximum limit to prevent excessive queries
		s.logger.DebugContext(ctx, "Limit exceeded maximum value, capping", "chat_id", chatID, "capped_limit", limit)
	}

	// Check if context is already done
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var messages []Message
	query := `
        SELECT id, chat_id, user_id, content, timestamp, created_at, updated_at, processed_at
        FROM messages
        WHERE chat_id = ?
        ORDER BY timestamp DESC
        LIMIT ?;
    `

	s.logger.DebugContext(ctx, "Fetching recent messages", "chat_id", chatID, "limit", limit)
	err := s.db.SelectContext(ctx, &messages, query, chatID, limit)

	// Check for timeout or cancellation
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		s.logger.WarnContext(ctx, "Context timeout or cancellation while fetching messages",
			"chat_id", chatID, "error", err)
		return nil, err
	}

	if err != nil {
		s.logger.ErrorContext(ctx, "Error getting recent messages", "chat_id", chatID, "limit", limit, "error", err)
		return nil, fmt.Errorf("failed to get recent messages for chat %d: %w", chatID, err)
	}

	s.logger.DebugContext(ctx, "Fetched recent messages successfully", "chat_id", chatID, "count", len(messages))
	return messages, nil
}

// RunSQLMaintenance executes a VACUUM command on the SQLite database.
// Improved with context timeout handling and better error messages.
func (s *sqlxStore) RunSQLMaintenance(ctx context.Context) error {
	// Check if context is already done
	if ctx.Err() != nil {
		s.logger.WarnContext(ctx, "Context cancelled or timed out before starting VACUUM", "error", ctx.Err())
		return ctx.Err()
	}

	s.logger.InfoContext(ctx, "Starting database maintenance (VACUUM)...")

	// Start a maintenance transaction - SQLite requires VACUUM to run outside a transaction
	// but we can prepare and clean up within one
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to begin transaction for maintenance preparation", "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// First check if any other operations are in progress by checking for open transactions
	// This is SQLite specific - may need adjustment for other database types
	var openTxCount int
	err = tx.GetContext(ctx, &openTxCount, "PRAGMA busy_timeout = 5000;") // Set busy timeout to 5 seconds
	if err != nil {
		s.logger.WarnContext(ctx, "Failed to set busy timeout", "error", err)
		_ = tx.Rollback() // Ignore potential rollback error
	} else {
		// Commit this initial transaction before VACUUM
		if err := tx.Commit(); err != nil {
			s.logger.WarnContext(ctx, "Failed to commit busy_timeout settings", "error", err)
		}
	}

	// Execute VACUUM - must be outside a transaction in SQLite
	_, err = s.db.ExecContext(ctx, "VACUUM;")

	// Check specific cases
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		s.logger.WarnContext(ctx, "VACUUM operation timed out or was cancelled", "error", err)
		return fmt.Errorf("database maintenance (VACUUM) timed out: %w", err)

	case err != nil:
		s.logger.ErrorContext(ctx, "Database maintenance (VACUUM) failed", "error", err)
		return fmt.Errorf("failed to execute VACUUM: %w", err)

	default:
		s.logger.InfoContext(ctx, "Database maintenance (VACUUM) completed successfully")
	}

	return nil
}

// --- Implementations for ported features ---

// GetUserProfile retrieves a user profile by user ID. Returns nil, nil if not found.
// Improved with parameter validation and context timeout checks.
func (s *sqlxStore) GetUserProfile(ctx context.Context, userID int64) (*UserProfile, error) {
	// Validate parameters
	if userID == 0 {
		return nil, fmt.Errorf("user_id cannot be zero")
	}

	// Check if context is already done
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var profile UserProfile
	query := `SELECT id, created_at, updated_at, user_id, aliases, origin_location, current_location, age_range, traits
	          FROM user_profiles WHERE user_id = ?`

	err := s.db.GetContext(ctx, &profile, query, userID)

	// Handle specific error cases
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Not found is expected in some cases, not an error
		s.logger.DebugContext(ctx, "No user profile found", "user_id", userID)
		return nil, nil

	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		s.logger.WarnContext(ctx, "Context timeout or cancellation while fetching user profile",
			"user_id", userID, "error", err)
		return nil, err

	case err != nil:
		s.logger.ErrorContext(ctx, "Error getting user profile by ID", "user_id", userID, "error", err)
		return nil, fmt.Errorf("failed to get user profile for user ID %d: %w", userID, err)
	}

	s.logger.DebugContext(ctx, "Successfully retrieved user profile", "user_id", userID)
	return &profile, nil
}

// GetAllUserProfiles retrieves all user profiles.
// Improved with context timeout handling and better error reporting.
func (s *sqlxStore) GetAllUserProfiles(ctx context.Context) (map[int64]*UserProfile, error) {
	// Check if context is already done
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var profiles []*UserProfile // Fetch as slice first
	query := `SELECT id, created_at, updated_at, user_id, aliases, origin_location, current_location, age_range, traits
	          FROM user_profiles`

	s.logger.DebugContext(ctx, "Fetching all user profiles")
	err := s.db.SelectContext(ctx, &profiles, query)

	// Handle specific error cases
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		s.logger.WarnContext(ctx, "Context timeout or cancellation while fetching all user profiles", "error", err)
		return nil, err

	case err != nil:
		s.logger.ErrorContext(ctx, "Error getting all user profiles", "error", err)
		return nil, fmt.Errorf("failed to get all user profiles: %w", err)
	}

	// Convert slice to map
	profileMap := make(map[int64]*UserProfile, len(profiles))
	for _, p := range profiles {
		if p != nil { // Defensive check to avoid nil pointer dereference
			profileMap[p.UserID] = p
		} else {
			s.logger.WarnContext(ctx, "Found nil profile in database results")
		}
	}

	s.logger.DebugContext(ctx, "Successfully fetched all user profiles", "count", len(profiles))
	return profileMap, nil
}

// SaveUserProfile inserts or updates a user profile based on UserID.
// Uses a transaction to ensure atomicity and better error handling for SQLite dialect differences.
func (s *sqlxStore) SaveUserProfile(ctx context.Context, profile *UserProfile) error {
	if profile == nil {
		return fmt.Errorf("cannot save nil user profile")
	}

	now := time.Now().UTC()
	profile.UpdatedAt = now

	// If profile's CreatedAt is zero, this is a new profile
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}

	// Start a transaction
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to begin transaction for saving user profile",
			"user_id", profile.UserID, "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				if !errors.Is(rollbackErr, sql.ErrTxDone) {
					s.logger.WarnContext(ctx, "Error rolling back transaction", "error", rollbackErr)
				}
			}
		}
	}()

	// First check if the profile exists
	var exists bool
	err = tx.GetContext(ctx, &exists,
		`SELECT 1 FROM user_profiles WHERE user_id = ? LIMIT 1`, profile.UserID)

	var result sql.Result

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.logger.ErrorContext(ctx, "Error checking if profile exists",
			"user_id", profile.UserID, "error", err)
		return fmt.Errorf("failed to check if profile exists for user ID %d: %w", profile.UserID, err)
	}

	if exists {
		// Update existing profile
		query := `
			UPDATE user_profiles SET
				aliases = :aliases,
				origin_location = :origin_location,
				current_location = :current_location,
				age_range = :age_range,
				traits = :traits,
				updated_at = :updated_at
			WHERE user_id = :user_id
		`
		result, err = tx.NamedExecContext(ctx, query, profile)
	} else {
		// Insert new profile
		query := `
			INSERT INTO user_profiles (
				user_id, aliases, origin_location, current_location,
				age_range, traits, created_at, updated_at
			) VALUES (
				:user_id, :aliases, :origin_location, :current_location,
				:age_range, :traits, :created_at, :updated_at
			)
		`
		result, err = tx.NamedExecContext(ctx, query, profile)
	}

	if err != nil {
		s.logger.ErrorContext(ctx, "Error saving user profile",
			"user_id", profile.UserID, "error", err)
		return fmt.Errorf("failed to save user profile for user ID %d: %w", profile.UserID, err)
	}

	// Check that we affected exactly one row
	affected, err := result.RowsAffected()
	if err != nil {
		s.logger.WarnContext(ctx, "Could not get affected row count when saving profile",
			"user_id", profile.UserID, "error", err)
	} else if affected != 1 {
		s.logger.WarnContext(ctx, "Unexpected number of rows affected when saving profile",
			"user_id", profile.UserID, "affected", affected)
	}

	// If this is a new profile, get the auto-generated ID
	if !exists {
		id, err := result.LastInsertId()
		if err == nil {
			//nolint:gosec // integer overflow conversion is acceptable here
			profile.ID = uint(id)
		} else {
			s.logger.WarnContext(ctx, "Could not get last insert ID for user profile",
				"user_id", profile.UserID, "error", err)
		}
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		s.logger.ErrorContext(ctx, "Failed to commit transaction",
			"user_id", profile.UserID, "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	// Successfully committed, set tx to nil to avoid rollback
	tx = nil

	operation := "updated"
	if !exists {
		operation = "created"
	}
	s.logger.DebugContext(ctx, "User profile saved successfully",
		"operation", operation, "user_id", profile.UserID)

	return nil
}

// GetUnprocessedMessages retrieves messages that haven't been processed for profile analysis.
// Enhanced with context timeout handling and better error reporting.
func (s *sqlxStore) GetUnprocessedMessages(ctx context.Context) ([]*Message, error) {
	// Check if context is already done
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var messages []*Message
	// Select messages where processed_at is NULL
	query := `SELECT id, chat_id, user_id, content, timestamp, created_at, updated_at, processed_at
	          FROM messages
	          WHERE processed_at IS NULL
	          ORDER BY timestamp ASC` // Process in chronological order

	s.logger.DebugContext(ctx, "Fetching unprocessed messages")
	err := s.db.SelectContext(ctx, &messages, query)

	// Handle specific error cases
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		s.logger.WarnContext(ctx, "Context timeout or cancellation while fetching unprocessed messages", "error", err)
		return nil, err

	case err != nil:
		s.logger.ErrorContext(ctx, "Error getting unprocessed messages", "error", err)
		return nil, fmt.Errorf("failed to get unprocessed messages: %w", err)
	}

	s.logger.DebugContext(ctx, "Successfully fetched unprocessed messages", "count", len(messages))
	return messages, nil
}

// MarkMessagesAsProcessed marks a list of messages as processed by setting processed_at.
// Uses a transaction to ensure atomicity when updating multiple messages.
func (s *sqlxStore) MarkMessagesAsProcessed(ctx context.Context, messageIDs []uint) error {
	if len(messageIDs) == 0 {
		return nil // Nothing to mark
	}

	// Start a transaction for atomicity
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to begin transaction for marking messages", "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	// Defer a rollback in case of failure - this is a no-op if the transaction is committed
	defer func() {
		if tx != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				// Only log rollback errors if it's not due to "transaction already committed"
				if !errors.Is(rollbackErr, sql.ErrTxDone) {
					s.logger.WarnContext(ctx, "Error rolling back transaction", "error", rollbackErr)
				}
			}
		}
	}()

	now := time.Now().UTC()
	query, args, err := sqlx.In(`UPDATE messages SET processed_at = ? WHERE id IN (?)`, now, messageIDs)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error building query for marking messages", "error", err)
		return fmt.Errorf("failed to build query for marking messages: %w", err)
	}

	query = tx.Rebind(query) // Rebind for specific SQL driver within the transaction
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error marking messages as processed", "error", err)
		return fmt.Errorf("failed to mark messages as processed: %w", err)
	}

	// Verify that the expected number of rows were affected
	affected, err := result.RowsAffected()
	if err != nil {
		s.logger.WarnContext(ctx, "Could not get affected row count", "error", err)
	} else if int(affected) != len(messageIDs) {
		s.logger.WarnContext(ctx, "Not all messages were marked as processed",
			"requested", len(messageIDs),
			"affected", affected)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		s.logger.ErrorContext(ctx, "Failed to commit transaction", "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	// Successfully committed, set tx to nil to avoid rollback
	tx = nil

	s.logger.DebugContext(ctx, "Marked messages as processed successfully",
		"count", len(messageIDs),
		"affected", affected)
	return nil
}

// DeleteAllMessages deletes all messages (used by reset command).
func (s *sqlxStore) DeleteAllMessages(ctx context.Context) error {
	query := `DELETE FROM messages`
	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error deleting all messages", "error", err)
		return fmt.Errorf("failed to delete all messages: %w", err)
	}

	count, _ := result.RowsAffected()
	s.logger.InfoContext(ctx, "Deleted all messages", "count", count)
	return nil
}

// DeleteAllUserProfiles deletes all user profiles (used by reset command).
func (s *sqlxStore) DeleteAllUserProfiles(ctx context.Context) error {
	query := `DELETE FROM user_profiles`
	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error deleting all user profiles", "error", err)
		return fmt.Errorf("failed to delete all user profiles: %w", err)
	}

	count, _ := result.RowsAffected()
	s.logger.InfoContext(ctx, "Deleted all user profiles", "count", count)
	return nil
}

// GetRecentMessages retrieves recent messages from a specific chat, with filtering support.
// Fetches 'limit' messages with ID less than or equal to 'beforeID'.
// Results are ordered chronologically (timestamp DESC, id DESC).
// Improved with parameter validation and context timeout checks.
func (s *sqlxStore) GetRecentMessages(ctx context.Context, chatID int64, limit int, beforeID uint) ([]*Message, error) { // beforeTimestamp is no longer used in the query but kept for interface compatibility
	// Validate parameters
	if chatID == 0 {
		return nil, fmt.Errorf("chat_id cannot be zero")
	}

	// Use caller-provided limit without overriding it
	if limit <= 0 {
		limit = 20 // Only provide a default if the caller didn't specify one
		s.logger.DebugContext(ctx, "No limit provided, using default", "chat_id", chatID, "default_limit", limit)
	}

	var messages []*Message

	// Check if context is already done
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Use zero ID if it's the first fetch
	isFirstFetch := beforeID == 0 // Check only beforeID now
	if isFirstFetch {
		// A large number ensures the ID condition doesn't exclude the latest messages
		beforeID = ^uint(0) // Max uint value
	}

	// --- Change: Use only ID for WHERE clause, keep ORDER BY timestamp, id ---
	query := `
        SELECT id, chat_id, user_id, content, timestamp, created_at, updated_at, processed_at
        FROM messages
        WHERE chat_id = ? AND id <= ?
        ORDER BY timestamp DESC, id DESC
        LIMIT ?;
    `

	s.logger.DebugContext(ctx, "Fetching messages with filters",
		"chat_id", chatID,
		"limit", limit,
		"before_id", beforeID)

	// Pass only necessary arguments for the simplified query
	err := s.db.SelectContext(ctx, &messages, query, chatID, beforeID, limit)

	// Check for timeout or cancellation
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		s.logger.WarnContext(ctx, "Context timeout or cancellation while fetching messages",
			"chat_id", chatID, "error", err)
		return nil, err
	}

	if err != nil {
		s.logger.ErrorContext(ctx, "Error getting messages with filters",
			"chat_id", chatID,
			"limit", limit,
			"before_id", beforeID,
			"error", err)
		return nil, fmt.Errorf("failed to get messages for chat %d: %w", chatID, err)
	}

	s.logger.DebugContext(ctx, "Fetched messages successfully",
		"chat_id", chatID,
		"count", len(messages))

	return messages, nil
}

// DeleteAllMessagesAndProfiles deletes all messages and user profiles in a single transaction.
// This ensures that either all data is deleted or none is (atomicity).
func (s *sqlxStore) DeleteAllMessagesAndProfiles(ctx context.Context) error {
	// Start a transaction for atomicity
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to begin transaction for data reset", "error", err)
		return fmt.Errorf("failed to begin transaction for data reset: %w", err)
	}
	// Defer a rollback in case of failure - this is a no-op if the transaction is committed
	defer func() {
		if tx != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				// Only log rollback errors if it's not due to "transaction already committed"
				if !errors.Is(rollbackErr, sql.ErrTxDone) {
					s.logger.WarnContext(ctx, "Error rolling back transaction", "error", rollbackErr)
				}
			}
		}
	}()

	// First, delete all messages
	messagesQuery := `DELETE FROM messages`
	messagesResult, err := tx.ExecContext(ctx, messagesQuery)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error deleting messages during reset", "error", err)
		return fmt.Errorf("failed to delete messages during reset: %w", err)
	}
	messagesCount, _ := messagesResult.RowsAffected()

	// Then, delete all profiles
	profilesQuery := `DELETE FROM user_profiles`
	profilesResult, err := tx.ExecContext(ctx, profilesQuery)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error deleting user profiles during reset", "error", err)
		return fmt.Errorf("failed to delete user profiles during reset: %w", err)
	}
	profilesCount, _ := profilesResult.RowsAffected()

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		s.logger.ErrorContext(ctx, "Failed to commit transaction for data reset", "error", err)
		return fmt.Errorf("failed to commit data reset transaction: %w", err)
	}
	// Successfully committed, set tx to nil to avoid rollback
	tx = nil

	s.logger.InfoContext(ctx, "Successfully reset all data",
		"messages_deleted", messagesCount,
		"profiles_deleted", profilesCount)
	return nil
}
