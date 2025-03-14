package db

import (
	"sort"
	"time"

	"github.com/edgard/murailobot/internal/errs"
	"github.com/edgard/murailobot/internal/logging"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// New creates a SQLite database connection.
//
//nolint:ireturn // Interface return is intentional for better abstraction
func New() (Database, error) {
	gormCfg := &gorm.Config{
		Logger: logging.NewGormLogger(),
	}

	db, err := gorm.Open(sqlite.Open("storage.db"), gormCfg)
	if err != nil {
		return nil, errs.NewDatabaseError("failed to open database", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, errs.NewDatabaseError("failed to get database instance", err)
	}

	// Use default max open connections value from constants
	sqlDB.SetMaxOpenConns(defaultMaxOpenConn)

	if err := db.AutoMigrate(&GroupMessage{}, &UserProfile{}); err != nil {
		return nil, errs.NewDatabaseError("failed to run migrations", err)
	}

	return &sqliteDB{
		db: db,
	}, nil
}

// SaveGroupMessage stores a message from a group chat.
func (d *sqliteDB) SaveGroupMessage(groupID int64, groupName string, userID int64, message string) error {
	groupMsg := GroupMessage{
		GroupID:   groupID,
		GroupName: groupName,
		UserID:    userID,
		Message:   message,
		Timestamp: time.Now().UTC(),
	}

	// Use a transaction for consistency
	err := d.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&groupMsg).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return errs.NewDatabaseError("failed to save group message", err)
	}

	return nil
}

// GetRecentGroupMessages retrieves the most recent group messages from a specific group chat.
func (d *sqliteDB) GetRecentGroupMessages(groupID int64, limit int) ([]GroupMessage, error) {
	if limit <= 0 {
		return nil, errs.NewValidationError("invalid limit", nil)
	}

	var groupMsgs []GroupMessage
	if err := d.db.Where("group_id = ?", groupID).
		Order("timestamp desc").
		Limit(limit).
		Find(&groupMsgs).Error; err != nil {
		return nil, errs.NewDatabaseError("failed to get recent group messages", err)
	}

	// Sort messages chronologically (oldest first) before returning
	// This helps consumers avoid having to sort themselves
	sort.Slice(groupMsgs, func(i, j int) bool {
		return groupMsgs[i].Timestamp.Before(groupMsgs[j].Timestamp)
	})

	return groupMsgs, nil
}

// GetGroupMessagesInTimeRange retrieves all group messages within a time range.
func (d *sqliteDB) GetGroupMessagesInTimeRange(start, end time.Time) ([]GroupMessage, error) {
	if err := validateTimeRange(start, end); err != nil {
		return nil, errs.NewValidationError("invalid time range", err)
	}

	var groupMsgs []GroupMessage
	if err := d.db.Where("timestamp >= ? AND timestamp < ?", start, end).
		Order("timestamp asc").
		Find(&groupMsgs).Error; err != nil {
		return nil, errs.NewDatabaseError("failed to get group messages", err)
	}

	return groupMsgs, nil
}

// GetAllGroupMessages retrieves all group messages stored in the database.
func (d *sqliteDB) GetAllGroupMessages() ([]GroupMessage, error) {
	var groupMsgs []GroupMessage

	// We'll order by timestamp to process messages chronologically
	if err := d.db.Order("timestamp asc").
		Find(&groupMsgs).Error; err != nil {
		return nil, errs.NewDatabaseError("failed to get all group messages", err)
	}

	return groupMsgs, nil
}

// GetUnprocessedGroupMessages retrieves all group messages that have not been processed yet.
func (d *sqliteDB) GetUnprocessedGroupMessages() ([]GroupMessage, error) {
	var groupMsgs []GroupMessage

	// Get only messages where processed_at is NULL, ordered chronologically
	if err := d.db.Where("processed_at IS NULL").
		Order("timestamp asc").
		Find(&groupMsgs).Error; err != nil {
		return nil, errs.NewDatabaseError("failed to get unprocessed group messages", err)
	}

	return groupMsgs, nil
}

// MarkGroupMessagesAsProcessed marks a batch of group messages as processed.
func (d *sqliteDB) MarkGroupMessagesAsProcessed(messageIDs []uint) error {
	if len(messageIDs) == 0 {
		return errs.NewValidationError("empty message IDs", nil)
	}

	now := time.Now().UTC()

	// Use a transaction to ensure consistency
	err := d.db.Transaction(func(tx *gorm.DB) error {
		// Process in smaller batches to avoid potential issues with large IN clauses
		batchSize := 100
		for i := 0; i < len(messageIDs); i += batchSize {
			end := i + batchSize
			if end > len(messageIDs) {
				end = len(messageIDs)
			}

			batch := messageIDs[i:end]
			if err := tx.Model(&GroupMessage{}).
				Where("id IN ?", batch).
				Update("processed_at", now).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return errs.NewDatabaseError("failed to mark messages as processed", err)
	}

	return nil
}

// DeleteProcessedGroupMessages deletes processed messages older than the cutoff time.
func (d *sqliteDB) DeleteProcessedGroupMessages(cutoffTime time.Time) error {
	if cutoffTime.IsZero() {
		return errs.NewValidationError("zero cutoff time", nil)
	}

	// Only delete messages that have been processed (non-nil processed_at)
	// and whose processed_at timestamp is older than the cutoff time
	if err := d.db.Where("processed_at IS NOT NULL AND processed_at < ?", cutoffTime).
		Delete(&GroupMessage{}).Error; err != nil {
		return errs.NewDatabaseError("failed to delete processed messages", err)
	}

	return nil
}

// DeleteProcessedGroupMessagesExcept deletes processed messages for a specific group chat
// while preserving the messages with IDs in the preserveIDs list.
func (d *sqliteDB) DeleteProcessedGroupMessagesExcept(groupID int64, cutoffTime time.Time, preserveIDs []uint) error {
	if cutoffTime.IsZero() {
		return errs.NewValidationError("zero cutoff time", nil)
	}

	if groupID <= 0 {
		return errs.NewValidationError("invalid group ID", nil)
	}

	// Log warning if no IDs are being preserved - this might be intentional, but worth noting
	if len(preserveIDs) == 0 {
		logging.Warn("no message IDs preserved in DeleteProcessedGroupMessagesExcept",
			"group_id", groupID,
			"cutoff_time", cutoffTime.Format(time.RFC3339))
	}

	// Only proceed if there are messages to delete
	// Use a transaction to ensure data consistency
	err := d.db.Transaction(func(tx *gorm.DB) error {
		// Build the query: delete processed messages for this group before cutoff time,
		// except those with IDs in preserveIDs
		query := tx.Where("group_id = ? AND processed_at IS NOT NULL AND processed_at < ?",
			groupID, cutoffTime)

		// If we have IDs to preserve, add them to the query
		// Process in smaller batches to avoid potential issues with large NOT IN clauses
		if len(preserveIDs) > 0 {
			// For small lists, use a single query for efficiency
			if len(preserveIDs) <= 100 {
				query = query.Where("id NOT IN ?", preserveIDs)

				// Execute the delete with a single query
				if err := query.Delete(&GroupMessage{}).Error; err != nil {
					return err
				}
			} else {
				// For larger lists, find eligible IDs first, then filter out preserved IDs in Go
				// to avoid SQLite limitations with large parameter lists
				var messagesToCheck []GroupMessage
				if err := tx.Where("group_id = ? AND processed_at IS NOT NULL AND processed_at < ?",
					groupID, cutoffTime).Select("id").Find(&messagesToCheck).Error; err != nil {
					return err
				}

				// Convert preserveIDs to map for O(1) lookups
				preserveMap := make(map[uint]struct{}, len(preserveIDs))
				for _, id := range preserveIDs {
					preserveMap[id] = struct{}{}
				}

				// Filter out preserved IDs
				var idsToDelete []uint

				for _, msg := range messagesToCheck {
					if _, exists := preserveMap[msg.ID]; !exists {
						idsToDelete = append(idsToDelete, msg.ID)
					}
				}

				// Delete in batches to avoid large IN clauses
				batchSize := 100
				for i := 0; i < len(idsToDelete); i += batchSize {
					end := i + batchSize
					if end > len(idsToDelete) {
						end = len(idsToDelete)
					}

					batch := idsToDelete[i:end]
					if len(batch) > 0 {
						if err := tx.Where("id IN ?", batch).Delete(&GroupMessage{}).Error; err != nil {
							return err
						}
					}
				}
			}
		} else {
			// No IDs to preserve, execute a simple delete
			if err := query.Delete(&GroupMessage{}).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return errs.NewDatabaseError("failed to delete processed messages with exceptions", err)
	}

	return nil
}

// GetUniqueGroupChats returns the IDs of all distinct group chats in the database.
func (d *sqliteDB) GetUniqueGroupChats() ([]int64, error) {
	var groupIDs []int64

	// Query for distinct group_id values
	if err := d.db.Model(&GroupMessage{}).
		Distinct("group_id").
		Pluck("group_id", &groupIDs).Error; err != nil {
		return nil, errs.NewDatabaseError("failed to get unique group chats", err)
	}

	return groupIDs, nil
}

const maxTimeRange = 31 * 24 * time.Hour // Maximum 31 days range

// validateTimeRange ensures the time range is valid.
func validateTimeRange(start, end time.Time) error {
	if start.IsZero() || end.IsZero() {
		return errs.NewValidationError("zero time value", nil)
	}

	if start.After(end) {
		return errs.NewValidationError("invalid time range: start after end", nil)
	}

	if end.Sub(start) > maxTimeRange {
		return errs.NewValidationError("time range exceeds maximum", nil)
	}

	return nil
}

// DeleteAll removes all stored data in a single transaction.
func (d *sqliteDB) DeleteAll() error {
	if err := d.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&GroupMessage{}).Error; err != nil {
			return errs.NewDatabaseError("failed to delete group messages", err)
		}

		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&UserProfile{}).Error; err != nil {
			return errs.NewDatabaseError("failed to delete user profiles", err)
		}

		return nil
	}); err != nil {
		return errs.NewDatabaseError("transaction failed during delete all", err)
	}

	return nil
}

// GetUserProfile retrieves a user's profile by user ID.
func (d *sqliteDB) GetUserProfile(userID int64) (*UserProfile, error) {
	var profile UserProfile
	result := d.db.Where("user_id = ?", userID).First(&profile)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil // Profile doesn't exist yet
		}

		return nil, errs.NewDatabaseError("failed to get user profile", result.Error)
	}

	return &profile, nil
}

// SaveUserProfile creates or updates a user profile.
func (d *sqliteDB) SaveUserProfile(profile *UserProfile) error {
	if profile == nil {
		return errs.NewValidationError("nil profile", nil)
	}

	// Validate UserID
	if profile.UserID <= 0 {
		return errs.NewValidationError("invalid user ID", nil)
	}

	// Use a transaction to ensure data consistency
	err := d.db.Transaction(func(tx *gorm.DB) error {
		// Check if profile exists
		var existingProfile UserProfile
		result := tx.Where("user_id = ?", profile.UserID).First(&existingProfile)

		if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
			return result.Error
		}

		// Set the last updated time
		profile.LastUpdated = time.Now().UTC()

		// If profile exists, update it
		if result.Error == nil {
			profile.ID = existingProfile.ID
			profile.CreatedAt = existingProfile.CreatedAt

			if err := tx.Save(profile).Error; err != nil {
				return err
			}

			return nil
		}

		// Otherwise create a new profile
		if err := tx.Create(profile).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return errs.NewDatabaseError("failed to save user profile", err)
	}

	return nil
}

// GetAllUserProfiles retrieves all user profiles.
func (d *sqliteDB) GetAllUserProfiles() (map[int64]*UserProfile, error) {
	var profiles []UserProfile
	if err := d.db.Find(&profiles).Error; err != nil {
		return nil, errs.NewDatabaseError("failed to get all user profiles", err)
	}

	// Map profiles by user ID for easy lookup
	result := make(map[int64]*UserProfile, len(profiles))
	for i := range profiles {
		result[profiles[i].UserID] = &profiles[i]
	}

	return result, nil
}

// Close ensures all pending operations are completed and resources are released.
func (d *sqliteDB) Close() error {
	sqlDB, err := d.db.DB()
	if err != nil {
		return errs.NewDatabaseError("failed to get database instance", err)
	}

	if err := sqlDB.Close(); err != nil {
		return errs.NewDatabaseError("failed to close database", err)
	}

	return nil
}
