// Package db provides database models and operations for MurailoBot,
// handling message storage, user profile management, and data persistence.
package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/edgard/murailobot/internal/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB provides database operations for storing and retrieving messages
// and user profiles using SQLite as the underlying storage engine.
type DB struct {
	db *gorm.DB
}

// New creates a new database instance with the provided configuration.
// It initializes the SQLite database, configures connection settings,
// and runs any necessary migrations.
//
// Returns an error if database initialization fails.
func New(cfg *config.Config) (*DB, error) {
	startTime := time.Now()
	slog.Debug("initializing database", "path", cfg.DBPath)

	// Configure GORM logger
	gormLogger := logger.New(
		&gormLogAdapter{},
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	gormConfig := &gorm.Config{
		Logger: gormLogger,
	}

	// Open database connection and configure pool
	dbOpenStart := time.Now()
	db, err := gorm.Open(sqlite.Open(cfg.DBPath), gormConfig)
	if err != nil {
		slog.Error("failed to open database",
			"error", err,
			"path", cfg.DBPath,
			"duration_ms", time.Since(dbOpenStart).Milliseconds())
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Get SQL DB instance for connection pool configuration
	sqlDB, err := db.DB()
	if err != nil {
		slog.Error("failed to get database instance", "error", err)
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(1)                // Single connection for SQLite
	sqlDB.SetMaxIdleConns(1)                // Keep connection idle in pool
	sqlDB.SetConnMaxLifetime(1 * time.Hour) // Recycle after 1 hour

	slog.Debug("database connection configured",
		"path", cfg.DBPath,
		"duration_ms", time.Since(dbOpenStart).Milliseconds())

	// Run migrations
	migrationStart := time.Now()
	if err := db.AutoMigrate(&Message{}, &UserProfile{}); err != nil {
		slog.Error("failed to run migrations",
			"error", err,
			"duration_ms", time.Since(migrationStart).Milliseconds())
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	totalDuration := time.Since(startTime)
	slog.Info("database initialization complete",
		"duration_ms", totalDuration.Milliseconds(),
		"migration_ms", time.Since(migrationStart).Milliseconds())

	return &DB{
		db: db,
	}, nil
}

// SaveMessage stores a new message from a group chat in the database.
// Returns an error if the message is nil or if the database operation fails.
func (r *DB) SaveMessage(ctx context.Context, msg *Message) error {
	if msg == nil {
		return errors.New("nil message")
	}

	// Only measure timing if we're going to log it (slow operation)
	startTime := time.Now()

	err := r.db.WithContext(ctx).Create(msg).Error
	if err != nil {
		return fmt.Errorf("failed to save message for user %d: %w", msg.UserID, err)
	}

	// Only log slow operations to reduce noise
	slowThreshold := 100 * time.Millisecond
	duration := time.Since(startTime)
	if duration > slowThreshold {
		slog.Warn("slow database operation detected",
			"operation", "save_message",
			"group_id", msg.GroupID,
			"duration_ms", duration.Milliseconds())
	}

	return nil
}

// GetRecentMessages retrieves messages from a specific group chat with combined timestamp and ID-based pagination.
// Messages are returned in chronological order (oldest first).
//
// Parameters:
// - groupID: The Telegram group chat ID
// - limit: Maximum number of messages to retrieve per batch
// - beforeTimestamp: Only retrieve messages with timestamps before or equal to this value
// - beforeID: Only retrieve messages with IDs less than this value (used to break timestamp ties)
//
// Returns an error if the parameters are invalid or if the database operation fails.
func (r *DB) GetRecentMessages(ctx context.Context, groupID int64, limit int, beforeTimestamp time.Time, beforeID uint) ([]*Message, error) {
	if limit <= 0 {
		return nil, errors.New("invalid limit")
	}

	query := r.db.WithContext(ctx).
		Where("group_id = ?", groupID).
		Order("timestamp desc, id desc").
		Limit(limit)

	// Apply timestamp and ID filters if provided
	if !beforeTimestamp.IsZero() {
		if beforeID > 0 {
			// When both timestamp and ID are provided
			query = query.Where("(timestamp < ?) OR (timestamp = ? AND id < ?)",
				beforeTimestamp, beforeTimestamp, beforeID)
		} else {
			// When only timestamp is provided
			query = query.Where("timestamp <= ?", beforeTimestamp)
		}
	} else if beforeID > 0 {
		// When only ID is provided
		query = query.Where("id < ?", beforeID)
	}

	// Query the database
	var messages []*Message
	if err := query.Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get recent messages: %w", err)
	}

	// Sort messages chronologically
	sort.Slice(messages, func(i, j int) bool {
		// First sort by timestamp
		if !messages[i].Timestamp.Equal(messages[j].Timestamp) {
			return messages[i].Timestamp.Before(messages[j].Timestamp)
		}
		// If timestamps are equal, sort by ID for consistent ordering
		return messages[i].ID < messages[j].ID
	})

	// Only log if an unusual number of messages is retrieved
	if len(messages) == 0 || len(messages) == limit {
		slog.Debug("messages retrieved",
			"group_id", groupID,
			"count", len(messages),
			"before_timestamp", beforeTimestamp,
			"before_id", beforeID)
	}

	return messages, nil
}

// GetMessagesInTimeRange retrieves all messages within a specified time range.
// The time range must be valid (start before end) and cannot exceed 31 days.
//
// Returns an error if the time range is invalid or if the database operation fails.
func (r *DB) GetMessagesInTimeRange(ctx context.Context, start, end time.Time) ([]*Message, error) {
	if start.IsZero() || end.IsZero() {
		return nil, errors.New("zero time value")
	}

	if start.After(end) {
		return nil, errors.New("invalid time range: start after end")
	}

	if end.Sub(start) > 31*24*time.Hour {
		return nil, errors.New("time range exceeds maximum")
	}

	var messages []*Message
	if err := r.db.WithContext(ctx).
		Where("timestamp >= ? AND timestamp < ?", start, end).
		Order("timestamp asc").
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get messages in time range: %w", err)
	}

	return messages, nil
}

// GetAllMessages retrieves all messages stored in the database,
// ordered chronologically by timestamp.
//
// Returns an error if the database operation fails.
func (r *DB) GetAllMessages(ctx context.Context) ([]*Message, error) {
	var messages []*Message

	if err := r.db.WithContext(ctx).
		Order("timestamp asc").
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get all messages: %w", err)
	}

	return messages, nil
}

// GetUnprocessedMessages retrieves all messages that have not been processed yet
// for user profile analysis, ordered chronologically by timestamp.
//
// Returns an error if the database operation fails.
func (r *DB) GetUnprocessedMessages(ctx context.Context) ([]*Message, error) {
	var messages []*Message

	if err := r.db.WithContext(ctx).
		Where("processed_at IS NULL").
		Order("timestamp asc").
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get unprocessed messages: %w", err)
	}

	return messages, nil
}

// MarkMessagesAsProcessed updates the processed_at timestamp for a batch of messages.
// Messages are processed in batches to avoid issues with large IN clauses.
//
// Returns an error if the message ID list is empty or if the database operation fails.
func (r *DB) MarkMessagesAsProcessed(ctx context.Context, messageIDs []uint) error {
	if len(messageIDs) == 0 {
		return errors.New("empty message IDs")
	}

	now := time.Now().UTC()

	batchSize := 100
	for i := 0; i < len(messageIDs); i += batchSize {
		end := i + batchSize
		if end > len(messageIDs) {
			end = len(messageIDs)
		}

		batch := messageIDs[i:end]
		if err := r.db.WithContext(ctx).
			Model(&Message{}).
			Where("id IN ?", batch).
			Update("processed_at", now).Error; err != nil {
			return fmt.Errorf("failed to mark messages as processed: %w", err)
		}
	}

	return nil
}

// DeleteProcessedMessages deletes processed messages older than the cutoff time.
//
// Returns an error if the cutoff time is zero or if the database operation fails.
func (r *DB) DeleteProcessedMessages(ctx context.Context, cutoffTime time.Time) error {
	if cutoffTime.IsZero() {
		return errors.New("zero cutoff time")
	}

	if err := r.db.WithContext(ctx).
		Where("processed_at IS NOT NULL AND processed_at < ?", cutoffTime).
		Delete(&Message{}).Error; err != nil {
		return fmt.Errorf("failed to delete processed messages: %w", err)
	}

	return nil
}

// DeleteProcessedMessagesExcept deletes processed messages for a specific group chat
// while preserving the messages with IDs in the preserveIDs list.
//
// This method is used for message cleanup while maintaining conversation context.
//
// Returns an error if the parameters are invalid or if the database operation fails.
func (r *DB) DeleteProcessedMessagesExcept(ctx context.Context, groupID int64, cutoffTime time.Time, preserveIDs []uint) error {
	if cutoffTime.IsZero() {
		return errors.New("zero cutoff time")
	}

	if groupID <= 0 {
		return errors.New("invalid group ID")
	}

	// Prepare base query for deleting processed messages from a specific group
	query := r.db.WithContext(ctx).
		Where("group_id = ? AND processed_at IS NOT NULL AND processed_at < ?", groupID, cutoffTime)

	if len(preserveIDs) > 0 {
		// For small lists of IDs to preserve, we can use a simple NOT IN clause
		if len(preserveIDs) <= 100 {
			// SQLite has limits on the number of parameters in a query
			// This approach works well for smaller preserveIDs lists
			if err := query.Where("id NOT IN ?", preserveIDs).
				Delete(&Message{}).Error; err != nil {
				return fmt.Errorf("failed to delete processed messages with exceptions: %w", err)
			}
		} else {
			// For larger lists, we use a different approach to avoid SQL query size limitations
			// First, get all candidate messages that might be deleted
			var messagesToCheck []*Message
			if err := r.db.WithContext(ctx).
				Where("group_id = ? AND processed_at IS NOT NULL AND processed_at < ?", groupID, cutoffTime).
				Select("id").
				Find(&messagesToCheck).Error; err != nil {
				return fmt.Errorf("failed to get messages to check: %w", err)
			}

			// Create a map for O(1) lookups of preserved IDs
			preserveMap := make(map[uint]struct{}, len(preserveIDs))
			for _, id := range preserveIDs {
				preserveMap[id] = struct{}{}
			}

			// Identify which messages should be deleted (not in preserveIDs)
			var idsToDelete []uint
			for _, msg := range messagesToCheck {
				if _, exists := preserveMap[msg.ID]; !exists {
					idsToDelete = append(idsToDelete, msg.ID)
				}
			}

			// Delete in batches to avoid query size limitations
			batchSize := 100
			for i := 0; i < len(idsToDelete); i += batchSize {
				end := i + batchSize
				if end > len(idsToDelete) {
					end = len(idsToDelete)
				}

				batch := idsToDelete[i:end]
				if len(batch) > 0 {
					if err := r.db.WithContext(ctx).
						Where("id IN ?", batch).
						Delete(&Message{}).Error; err != nil {
						return fmt.Errorf("failed to delete batch of messages: %w", err)
					}
				}
			}
		}
	} else {
		// If no IDs to preserve, delete all matching messages
		if err := query.Delete(&Message{}).Error; err != nil {
			return fmt.Errorf("failed to delete processed messages: %w", err)
		}
	}

	return nil
}

// GetUniqueGroupChats returns the IDs of all distinct group chats in the database.
//
// Returns an error if the database operation fails.
func (r *DB) GetUniqueGroupChats(ctx context.Context) ([]int64, error) {
	var groupIDs []int64

	if err := r.db.WithContext(ctx).
		Model(&Message{}).
		Distinct("group_id").
		Pluck("group_id", &groupIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to get unique group chats: %w", err)
	}

	return groupIDs, nil
}

// GetUserProfile retrieves a user's profile by user ID.
// Returns nil without an error if the profile doesn't exist.
//
// Returns an error if the database operation fails.
func (r *DB) GetUserProfile(ctx context.Context, userID int64) (*UserProfile, error) {
	var profile UserProfile
	result := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&profile)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get user profile: %w", result.Error)
	}

	return &profile, nil
}

// SaveUserProfile creates a new user profile or updates an existing one.
// It automatically sets the LastUpdated timestamp and preserves the
// original CreatedAt time when updating.
//
// Returns an error if the profile is nil, has an invalid user ID,
// or if the database operation fails.
func (r *DB) SaveUserProfile(ctx context.Context, profile *UserProfile) error {
	if profile == nil {
		return errors.New("nil profile")
	}

	if profile.UserID <= 0 {
		return errors.New("invalid user ID")
	}

	// Set last updated timestamp
	profile.LastUpdated = time.Now().UTC()

	// Check if profile already exists
	var existingProfile UserProfile
	result := r.db.WithContext(ctx).Where("user_id = ?", profile.UserID).First(&existingProfile)

	var err error
	var isNew bool

	// Update or create based on existence
	if result.Error == nil {
		// Update existing profile - preserve metadata
		profile.ID = existingProfile.ID
		profile.CreatedAt = existingProfile.CreatedAt
		err = r.db.WithContext(ctx).Save(profile).Error
	} else if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// Create new profile
		isNew = true
		err = r.db.WithContext(ctx).Create(profile).Error
	} else {
		// Unexpected error
		return fmt.Errorf("failed to check existing profile: %w", result.Error)
	}

	if err != nil {
		return fmt.Errorf("failed to save user profile: %w", err)
	}

	// Only log at Info level for new profiles, Debug for updates
	if isNew {
		slog.Info("new user profile created", "user_id", profile.UserID)
	} else {
		slog.Debug("user profile updated", "user_id", profile.UserID)
	}

	return nil
}

// GetAllUserProfiles retrieves all user profiles and returns them as a map
// indexed by user ID for easy lookup.
//
// Returns an error if the database operation fails.
func (r *DB) GetAllUserProfiles(ctx context.Context) (map[int64]*UserProfile, error) {
	var profiles []*UserProfile
	if err := r.db.WithContext(ctx).Find(&profiles).Error; err != nil {
		return nil, fmt.Errorf("failed to get all user profiles: %w", err)
	}

	result := make(map[int64]*UserProfile, len(profiles))
	for _, profile := range profiles {
		result[profile.UserID] = profile
	}

	return result, nil
}

// DeleteAll removes all messages and user profiles from the database.
// This is typically used for resetting the bot's state.
//
// Returns an error if any of the delete operations fail.
func (r *DB) DeleteAll(ctx context.Context) error {
	if err := r.db.WithContext(ctx).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Message{}).Error; err != nil {
		return fmt.Errorf("failed to delete all messages: %w", err)
	}

	if err := r.db.WithContext(ctx).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&UserProfile{}).Error; err != nil {
		return fmt.Errorf("failed to delete all user profiles: %w", err)
	}

	return nil
}

// Close gracefully closes the database connection.
//
// Returns an error if closing the connection fails.
func (r *DB) Close() error {
	// Get database connection
	sqlDB, err := r.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	// Get connection stats before closing
	stats := sqlDB.Stats()

	// Only log warnings if there are issues with the connection pool
	if stats.OpenConnections > 5 || float64(stats.InUse)/float64(stats.OpenConnections+1) > 0.8 {
		slog.Warn("database connection pool pressure detected",
			"open_connections", stats.OpenConnections,
			"in_use", stats.InUse,
			"utilization_percent", float64(stats.InUse)/float64(stats.OpenConnections+1)*100)
	}

	// Close the connection
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	slog.Debug("database connection closed")
	return nil
}

type gormLogAdapter struct{}

func (l *gormLogAdapter) Printf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	slog.Debug("gorm", "message", msg)
}
