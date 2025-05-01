// Package database provides database setup, models, and data access layer (Store).
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
	// Deprecated: Use GetRecentMessages instead for more flexible history retrieval.
	GetRecentMessagesInChat(ctx context.Context, chatID int64, limit int) ([]Message, error)

	// RunSQLMaintenance performs database maintenance tasks like VACUUM.
	RunSQLMaintenance(ctx context.Context) error

	// GetUserProfile retrieves a user profile by user ID. Returns nil, nil if not found.
	GetUserProfile(ctx context.Context, userID int64) (*UserProfile, error)

	// GetAllUserProfiles retrieves all user profiles as a map keyed by UserID.
	GetAllUserProfiles(ctx context.Context) (map[int64]*UserProfile, error)

	// SaveUserProfile inserts or updates a user profile.
	SaveUserProfile(ctx context.Context, profile *UserProfile) error

	// GetUnprocessedMessages retrieves messages that haven't been processed for profile analysis.
	GetUnprocessedMessages(ctx context.Context) ([]*Message, error)

	// MarkMessagesAsProcessed marks a list of messages as processed by setting their processed_at timestamp.
	MarkMessagesAsProcessed(ctx context.Context, messageIDs []uint) error

	// DeleteAllMessages deletes all messages (used by reset command).
	// Deprecated: Use DeleteAllMessagesAndProfiles for atomic reset.
	DeleteAllMessages(ctx context.Context) error

	// DeleteAllUserProfiles deletes all user profiles (used by reset command).
	// Deprecated: Use DeleteAllMessagesAndProfiles for atomic reset.
	DeleteAllUserProfiles(ctx context.Context) error

	// GetRecentMessages retrieves recent messages from a specific chat, up to 'limit',
	// ending at or before 'beforeID'. Results are ordered newest first.
	GetRecentMessages(ctx context.Context, chatID int64, limit int, beforeID uint) ([]*Message, error)

	// DeleteAllMessagesAndProfiles deletes all messages and user profiles in a single transaction
	// to ensure atomicity for the reset operation.
	DeleteAllMessagesAndProfiles(ctx context.Context) error
}

// sqlxStore provides an implementation of the Store interface using sqlx.
type sqlxStore struct {
	db     *sqlx.DB
	logger *slog.Logger
}

// NewStore creates a new Store implementation backed by sqlx.
// It requires a connected sqlx.DB instance and a logger.
// If logger is nil, logging is discarded.
func NewStore(db *sqlx.DB, logger *slog.Logger) Store {
	if logger == nil {
		// Discard logs if no logger is provided
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

// SaveMessage inserts a new message record within a transaction.
func (s *sqlxStore) SaveMessage(ctx context.Context, message *Message) error {
	if message == nil {
		return fmt.Errorf("cannot save nil message")
	}
	// Basic validation of required fields
	if message.ChatID == 0 || message.UserID == 0 || message.Content == "" || message.Timestamp.IsZero() {
		return fmt.Errorf("message missing required fields (ChatID, UserID, Content, Timestamp)")
	}

	now := time.Now().UTC()
	message.CreatedAt = now
	message.UpdatedAt = now

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to begin transaction for saving message", "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	// Ensure rollback is attempted if commit fails or panics occur
	defer func() {
		if tx != nil {
			// Rollback() is safe to call even if Commit() succeeded.
			// We only log errors if the rollback itself failed unexpectedly.
			if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				s.logger.WarnContext(ctx, "Error rolling back transaction after SaveMessage failure", "error", rollbackErr)
			}
		}
	}()

	query := `
        INSERT INTO messages (chat_id, user_id, content, timestamp, created_at, updated_at, processed_at)
        VALUES (:chat_id, :user_id, :content, :timestamp, :created_at, :updated_at, :processed_at);
    `
	result, err := tx.NamedExecContext(ctx, query, message)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error executing save message query", "error", err)
		return fmt.Errorf("failed to save message (chat %d, user %d): %w", message.ChatID, message.UserID, err)
	}

	// Get the last inserted ID and update the message struct
	id, err := result.LastInsertId()
	if err == nil {
		//nolint:gosec // integer overflow conversion is acceptable here as DB IDs are unlikely to exceed uint max
		message.ID = uint(id)
	} else {
		// Log if getting LastInsertId fails, but don't fail the operation
		s.logger.WarnContext(ctx, "Could not retrieve last insert ID after saving message", "error", err)
	}

	// Verify row count
	affected, err := result.RowsAffected()
	if err == nil && affected != 1 {
		s.logger.WarnContext(ctx, "Unexpected number of rows affected when saving message", "affected", affected)
		// Potentially return an error here if 1 row affected is critical
	} else if err != nil {
		s.logger.WarnContext(ctx, "Could not retrieve affected row count after saving message", "error", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		s.logger.ErrorContext(ctx, "Failed to commit transaction for saving message", "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	tx = nil // Prevent deferred rollback after successful commit

	s.logger.DebugContext(ctx, "Message saved successfully", "message_id", message.ID)
	return nil
}

// GetRecentMessagesInChat retrieves the most recent 'limit' messages for a given chat ID.
// Deprecated: Use GetRecentMessages instead.
func (s *sqlxStore) GetRecentMessagesInChat(ctx context.Context, chatID int64, limit int) ([]Message, error) {
	s.logger.WarnContext(ctx, "Deprecated function GetRecentMessagesInChat called", "chat_id", chatID)
	if chatID == 0 {
		return nil, fmt.Errorf("chat_id cannot be zero")
	}
	if limit <= 0 {
		limit = 20 // Default limit
	} else if limit > 100 {
		limit = 100 // Cap limit
	}

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
	err := s.db.SelectContext(ctx, &messages, query, chatID, limit)

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		s.logger.WarnContext(ctx, "Context timeout/cancellation fetching recent messages (deprecated)", "error", err)
		return nil, err
	}
	if err != nil {
		s.logger.ErrorContext(ctx, "Error getting recent messages (deprecated)", "error", err)
		return nil, fmt.Errorf("failed to get recent messages for chat %d: %w", chatID, err)
	}
	return messages, nil
}

// RunSQLMaintenance executes a VACUUM command on the SQLite database to rebuild it and reduce fragmentation.
func (s *sqlxStore) RunSQLMaintenance(ctx context.Context) error {
	if ctx.Err() != nil {
		s.logger.WarnContext(ctx, "Context cancelled or timed out before starting VACUUM", "error", ctx.Err())
		return ctx.Err()
	}

	s.logger.InfoContext(ctx, "Starting database maintenance (VACUUM)...")

	// Set a busy timeout for the connection that will run VACUUM.
	// This helps if other (short-lived) reads are happening, but VACUUM needs exclusive access.
	// Note: VACUUM itself cannot be run inside an explicit transaction in SQLite.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to get connection for maintenance", "error", err)
		return fmt.Errorf("failed to get connection for maintenance: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			s.logger.WarnContext(ctx, "Error closing connection after maintenance", "error", closeErr)
		}
	}()

	// Setting busy_timeout on the connection before running VACUUM
	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout = 10000;"); err != nil { // 10 seconds timeout
		s.logger.WarnContext(ctx, "Failed to set busy_timeout for maintenance connection", "error", err)
		// Proceed anyway, but VACUUM might fail if the DB is busy
	}

	// Execute VACUUM on the specific connection
	_, err = conn.ExecContext(ctx, "VACUUM;")

	// Handle specific errors
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		s.logger.WarnContext(ctx, "VACUUM operation timed out or was cancelled", "error", err)
		return fmt.Errorf("database maintenance (VACUUM) timed out or cancelled: %w", err)
	case err != nil:
		s.logger.ErrorContext(ctx, "Database maintenance (VACUUM) failed", "error", err)
		return fmt.Errorf("failed to execute VACUUM: %w", err)
	default:
		s.logger.InfoContext(ctx, "Database maintenance (VACUUM) completed successfully")
	}

	return nil
}

// GetUserProfile retrieves a user profile by user ID. Returns nil, nil if not found.
func (s *sqlxStore) GetUserProfile(ctx context.Context, userID int64) (*UserProfile, error) {
	if userID == 0 {
		return nil, fmt.Errorf("user_id cannot be zero")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var profile UserProfile
	query := `SELECT id, created_at, updated_at, user_id, aliases, origin_location, current_location, age_range, traits
	          FROM user_profiles WHERE user_id = ?`
	err := s.db.GetContext(ctx, &profile, query, userID)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Not finding a profile is a normal condition, not an error.
		s.logger.DebugContext(ctx, "No user profile found", "user_id", userID)
		return nil, nil
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		s.logger.WarnContext(ctx, "Context timeout/cancellation fetching user profile", "error", err)
		return nil, err
	case err != nil:
		s.logger.ErrorContext(ctx, "Error getting user profile by ID", "error", err)
		return nil, fmt.Errorf("failed to get user profile for user ID %d: %w", userID, err)
	}

	s.logger.DebugContext(ctx, "Successfully retrieved user profile", "user_id", userID)
	return &profile, nil
}

// GetAllUserProfiles retrieves all user profiles as a map keyed by UserID.
func (s *sqlxStore) GetAllUserProfiles(ctx context.Context) (map[int64]*UserProfile, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var profiles []*UserProfile // Fetch as slice first for efficiency
	query := `SELECT id, created_at, updated_at, user_id, aliases, origin_location, current_location, age_range, traits
	          FROM user_profiles`
	s.logger.DebugContext(ctx, "Fetching all user profiles")
	err := s.db.SelectContext(ctx, &profiles, query)

	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		s.logger.WarnContext(ctx, "Context timeout/cancellation fetching all user profiles", "error", err)
		return nil, err
	case err != nil:
		s.logger.ErrorContext(ctx, "Error getting all user profiles", "error", err)
		return nil, fmt.Errorf("failed to get all user profiles: %w", err)
	}

	// Convert slice to map for easier lookup by UserID
	profileMap := make(map[int64]*UserProfile, len(profiles))
	for _, p := range profiles {
		if p != nil { // Defensive check
			profileMap[p.UserID] = p
		} else {
			s.logger.WarnContext(ctx, "Encountered nil profile pointer in GetAllUserProfiles result slice")
		}
	}

	s.logger.DebugContext(ctx, "Successfully fetched all user profiles", "count", len(profileMap))
	return profileMap, nil
}

// SaveUserProfile inserts or updates a user profile within a transaction.
// It checks if a profile exists for the UserID before deciding to INSERT or UPDATE.
func (s *sqlxStore) SaveUserProfile(ctx context.Context, profile *UserProfile) error {
	if profile == nil {
		return fmt.Errorf("cannot save nil user profile")
	}
	if profile.UserID == 0 {
		return fmt.Errorf("user profile must have a non-zero user_id")
	}

	now := time.Now().UTC()
	profile.UpdatedAt = now
	// Set CreatedAt only if it's zero (indicating a new profile)
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to begin transaction for saving user profile", "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				s.logger.WarnContext(ctx, "Error rolling back transaction after SaveUserProfile failure", "error", rollbackErr)
			}
		}
	}()

	// Check if the profile exists within the transaction
	var exists bool
	err = tx.GetContext(ctx, &exists, `SELECT 1 FROM user_profiles WHERE user_id = ? LIMIT 1`, profile.UserID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.logger.ErrorContext(ctx, "Error checking if profile exists", "error", err)
		return fmt.Errorf("failed to check if profile exists for user ID %d: %w", profile.UserID, err)
	}

	var result sql.Result
	operation := "update" // Assume update initially

	if exists {
		// Update existing profile
		query := `
			UPDATE user_profiles SET
				aliases = :aliases, origin_location = :origin_location, current_location = :current_location,
				age_range = :age_range, traits = :traits, updated_at = :updated_at
			WHERE user_id = :user_id
		`
		result, err = tx.NamedExecContext(ctx, query, profile)
	} else {
		// Insert new profile
		operation = "insert"
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
		s.logger.ErrorContext(ctx, "Error executing save user profile query", "operation", operation, "error", err)
		return fmt.Errorf("failed to %s user profile for user ID %d: %w", operation, profile.UserID, err)
	}

	// Verify row count
	affected, err := result.RowsAffected()
	if err != nil {
		s.logger.WarnContext(ctx, "Could not get affected row count when saving profile", "error", err)
	} else if affected != 1 {
		s.logger.WarnContext(ctx, "Unexpected number of rows affected when saving profile", "affected", affected)
	}

	// If inserting, update the profile struct with the generated ID
	if operation == "insert" {
		id, err := result.LastInsertId()
		if err == nil {
			//nolint:gosec // integer overflow conversion is acceptable here
			profile.ID = uint(id)
		} else {
			s.logger.WarnContext(ctx, "Could not get last insert ID for user profile", "error", err)
		}
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		s.logger.ErrorContext(ctx, "Failed to commit transaction for saving user profile", "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	tx = nil // Prevent deferred rollback

	s.logger.DebugContext(ctx, "User profile saved successfully", "operation", operation, "user_id", profile.UserID)
	return nil
}

// GetUnprocessedMessages retrieves messages where processed_at is NULL, ordered chronologically.
func (s *sqlxStore) GetUnprocessedMessages(ctx context.Context) ([]*Message, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var messages []*Message
	query := `SELECT id, chat_id, user_id, content, timestamp, created_at, updated_at, processed_at
	          FROM messages
	          WHERE processed_at IS NULL
	          ORDER BY timestamp ASC` // Process oldest first
	s.logger.DebugContext(ctx, "Fetching unprocessed messages")
	err := s.db.SelectContext(ctx, &messages, query)

	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		s.logger.WarnContext(ctx, "Context timeout/cancellation fetching unprocessed messages", "error", err)
		return nil, err
	case err != nil:
		s.logger.ErrorContext(ctx, "Error getting unprocessed messages", "error", err)
		return nil, fmt.Errorf("failed to get unprocessed messages: %w", err)
	}

	s.logger.DebugContext(ctx, "Successfully fetched unprocessed messages", "count", len(messages))
	return messages, nil
}

// MarkMessagesAsProcessed marks a list of messages as processed by setting processed_at within a transaction.
func (s *sqlxStore) MarkMessagesAsProcessed(ctx context.Context, messageIDs []uint) error {
	if len(messageIDs) == 0 {
		s.logger.DebugContext(ctx, "No message IDs provided to mark as processed")
		return nil // Nothing to do
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to begin transaction for marking messages", "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				s.logger.WarnContext(ctx, "Error rolling back transaction after MarkMessagesAsProcessed failure", "error", rollbackErr)
			}
		}
	}()

	now := time.Now().UTC()
	// Use sqlx.In to safely handle a variable number of IDs in the IN clause
	query, args, err := sqlx.In(`UPDATE messages SET processed_at = ? WHERE id IN (?)`, now, messageIDs)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error building query for marking messages", "error", err)
		return fmt.Errorf("failed to build query for marking messages: %w", err)
	}

	query = tx.Rebind(query) // Rebind query for the specific SQL driver (e.g., SQLite uses '?')
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error executing mark messages as processed query", "error", err)
		return fmt.Errorf("failed to mark messages as processed: %w", err)
	}

	// Verify that the expected number of rows were affected
	affected, err := result.RowsAffected()
	if err != nil {
		s.logger.WarnContext(ctx, "Could not get affected row count after marking messages", "error", err)
	} else if int(affected) != len(messageIDs) {
		// This might indicate some IDs didn't exist or were already processed (though the query doesn't check that)
		s.logger.WarnContext(ctx, "Number of messages marked processed differs from requested count",
			"requested", len(messageIDs), "affected", affected)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		s.logger.ErrorContext(ctx, "Failed to commit transaction for marking messages", "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	tx = nil // Prevent deferred rollback

	s.logger.DebugContext(ctx, "Marked messages as processed successfully", "count", affected)
	return nil
}

// DeleteAllMessages deletes all messages.
// Deprecated: Use DeleteAllMessagesAndProfiles for atomic reset.
func (s *sqlxStore) DeleteAllMessages(ctx context.Context) error {
	s.logger.WarnContext(ctx, "Deprecated function DeleteAllMessages called")
	query := `DELETE FROM messages`
	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error deleting all messages (deprecated)", "error", err)
		return fmt.Errorf("failed to delete all messages: %w", err)
	}
	count, _ := result.RowsAffected() // Ignore error getting count
	s.logger.InfoContext(ctx, "Deleted all messages (deprecated)", "count", count)
	return nil
}

// DeleteAllUserProfiles deletes all user profiles.
// Deprecated: Use DeleteAllMessagesAndProfiles for atomic reset.
func (s *sqlxStore) DeleteAllUserProfiles(ctx context.Context) error {
	s.logger.WarnContext(ctx, "Deprecated function DeleteAllUserProfiles called")
	query := `DELETE FROM user_profiles`
	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error deleting all user profiles (deprecated)", "error", err)
		return fmt.Errorf("failed to delete all user profiles: %w", err)
	}
	count, _ := result.RowsAffected() // Ignore error getting count
	s.logger.InfoContext(ctx, "Deleted all user profiles (deprecated)", "count", count)
	return nil
}

// GetRecentMessages retrieves recent messages from a specific chat, up to 'limit',
// ending at or before 'beforeID'. Results are ordered newest first (timestamp DESC, id DESC).
func (s *sqlxStore) GetRecentMessages(ctx context.Context, chatID int64, limit int, beforeID uint) ([]*Message, error) {
	if chatID == 0 {
		return nil, fmt.Errorf("chat_id cannot be zero")
	}
	// Use caller-provided limit, defaulting only if zero or negative
	if limit <= 0 {
		limit = 20 // Default limit if not specified
		s.logger.DebugContext(ctx, "No valid limit provided for GetRecentMessages, using default", "chat_id", chatID, "default_limit", limit)
	}
	// Consider capping the limit if necessary, e.g., limit = min(limit, 100)

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var messages []*Message
	// Use a very large number for beforeID if it's zero (first fetch)
	// This ensures the `id <= ?` condition doesn't exclude the latest messages.
	effectiveBeforeID := beforeID
	if beforeID == 0 {
		effectiveBeforeID = ^uint(0) // Max uint value
	}

	// Query fetches messages for the chat, with ID less than or equal to effectiveBeforeID,
	// ordered by timestamp then ID (both descending) to get the newest first, up to the limit.
	query := `
        SELECT id, chat_id, user_id, content, timestamp, created_at, updated_at, processed_at
        FROM messages
        WHERE chat_id = ? AND id <= ?
        ORDER BY timestamp DESC, id DESC
        LIMIT ?;
    `
	s.logger.DebugContext(ctx, "Fetching recent messages with filters",
		"chat_id", chatID, "limit", limit, "before_id", beforeID, "effective_before_id", effectiveBeforeID)
	err := s.db.SelectContext(ctx, &messages, query, chatID, effectiveBeforeID, limit)

	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		s.logger.WarnContext(ctx, "Context timeout/cancellation fetching recent messages", "error", err)
		return nil, err
	case err != nil:
		s.logger.ErrorContext(ctx, "Error getting recent messages with filters", "error", err)
		return nil, fmt.Errorf("failed to get messages for chat %d: %w", chatID, err)
	}

	s.logger.DebugContext(ctx, "Fetched recent messages successfully", "chat_id", chatID, "count", len(messages))
	return messages, nil
}

// DeleteAllMessagesAndProfiles deletes all messages and user profiles within a single transaction.
func (s *sqlxStore) DeleteAllMessagesAndProfiles(ctx context.Context) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to begin transaction for data reset", "error", err)
		return fmt.Errorf("failed to begin transaction for data reset: %w", err)
	}
	defer func() {
		if tx != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				s.logger.WarnContext(ctx, "Error rolling back transaction after DeleteAllMessagesAndProfiles failure", "error", rollbackErr)
			}
		}
	}()

	// Delete messages within the transaction
	messagesQuery := `DELETE FROM messages`
	messagesResult, err := tx.ExecContext(ctx, messagesQuery)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error deleting messages during reset transaction", "error", err)
		return fmt.Errorf("failed to delete messages during reset: %w", err)
	}
	messagesCount, _ := messagesResult.RowsAffected()

	// Delete profiles within the transaction
	profilesQuery := `DELETE FROM user_profiles`
	profilesResult, err := tx.ExecContext(ctx, profilesQuery)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error deleting user profiles during reset transaction", "error", err)
		return fmt.Errorf("failed to delete user profiles during reset: %w", err)
	}
	profilesCount, _ := profilesResult.RowsAffected()

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		s.logger.ErrorContext(ctx, "Failed to commit transaction for data reset", "error", err)
		return fmt.Errorf("failed to commit data reset transaction: %w", err)
	}
	tx = nil // Prevent deferred rollback

	s.logger.InfoContext(ctx, "Successfully reset all data (messages and profiles)",
		"messages_deleted", messagesCount, "profiles_deleted", profilesCount)
	return nil
}
