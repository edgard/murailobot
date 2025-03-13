package db

import (
	"time"

	errs "github.com/edgard/murailobot/internal/errors"
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

	sqlDB.SetMaxOpenConns(defaultMaxOpenConn)

	if err := db.AutoMigrate(&ChatHistory{}, &GroupMessage{}, &UserProfile{}); err != nil {
		return nil, errs.NewDatabaseError("failed to run migrations", err)
	}

	return &sqliteDB{
		db: db,
	}, nil
}

// GetRecent retrieves the most recent chat history entries.
func (d *sqliteDB) GetRecent(limit int) ([]ChatHistory, error) {
	if limit <= 0 {
		return nil, errs.NewValidationError("invalid limit", nil)
	}

	var chatHistory []ChatHistory
	if err := d.db.Order("timestamp desc").
		Limit(limit).
		Find(&chatHistory).Error; err != nil {
		return nil, errs.NewDatabaseError("failed to get recent history", err)
	}

	return chatHistory, nil
}

// Save stores a new chat interaction with the current UTC timestamp.
func (d *sqliteDB) Save(userID int64, userMsg, botMsg string) error {
	chatEntry := ChatHistory{
		UserID:    userID,
		UserMsg:   userMsg,
		BotMsg:    botMsg,
		Timestamp: time.Now().UTC(),
	}

	if err := d.db.Create(&chatEntry).Error; err != nil {
		return errs.NewDatabaseError("failed to save chat history", err)
	}

	return nil
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

	if err := d.db.Create(&groupMsg).Error; err != nil {
		return errs.NewDatabaseError("failed to save group message", err)
	}

	return nil
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

	// Update all messages in the batch with the current timestamp
	if err := d.db.Model(&GroupMessage{}).
		Where("id IN ?", messageIDs).
		Update("processed_at", now).Error; err != nil {
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

// DeleteChatHistory removes only the chat history, preserving user profiles and group messages.
func (d *sqliteDB) DeleteChatHistory() error {
	if err := d.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChatHistory{}).Error; err != nil {
		return errs.NewDatabaseError("failed to delete chat history", err)
	}

	return nil
}

// DeleteAll removes all stored data in a single transaction.
func (d *sqliteDB) DeleteAll() error {
	if err := d.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChatHistory{}).Error; err != nil {
			return errs.NewDatabaseError("failed to delete chat history", err)
		}

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

	// Check if profile exists
	existingProfile, err := d.GetUserProfile(profile.UserID)
	if err != nil {
		return errs.NewDatabaseError("failed to check existing profile", err)
	}

	// If profile exists, update it
	if existingProfile != nil {
		profile.ID = existingProfile.ID
		profile.CreatedAt = existingProfile.CreatedAt
		profile.LastUpdated = time.Now().UTC()

		if err := d.db.Save(profile).Error; err != nil {
			return errs.NewDatabaseError("failed to update user profile", err)
		}

		return nil
	}

	// Otherwise create a new profile
	profile.LastUpdated = time.Now().UTC()
	if err := d.db.Create(profile).Error; err != nil {
		return errs.NewDatabaseError("failed to create user profile", err)
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
