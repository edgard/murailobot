package db

import (
	"fmt"
	"time"

	"github.com/edgard/murailobot/internal/utils/logging"
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
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	sqlDB.SetMaxOpenConns(defaultMaxOpenConn)

	if err := db.AutoMigrate(&ChatHistory{}, &GroupMessage{}, &UserProfile{}); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &sqliteDB{
		db: db,
	}, nil
}

// GetRecent retrieves the most recent chat history entries.
func (d *sqliteDB) GetRecent(limit int) ([]ChatHistory, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidLimit, limit)
	}

	var chatHistory []ChatHistory
	if err := d.db.Order("timestamp desc").
		Limit(limit).
		Find(&chatHistory).Error; err != nil {
		return nil, fmt.Errorf("failed to get recent history: %w", err)
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
		return fmt.Errorf("failed to save chat history: %w", err)
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
		return fmt.Errorf("failed to save group message: %w", err)
	}

	return nil
}

// GetGroupMessagesInTimeRange retrieves all group messages within a time range.
func (d *sqliteDB) GetGroupMessagesInTimeRange(start, end time.Time) ([]GroupMessage, error) {
	if err := validateTimeRange(start, end); err != nil {
		return nil, fmt.Errorf("invalid time range: %w", err)
	}

	var groupMsgs []GroupMessage
	if err := d.db.Where("timestamp >= ? AND timestamp < ?", start, end).
		Order("timestamp asc").
		Find(&groupMsgs).Error; err != nil {
		return nil, fmt.Errorf("failed to get group messages: %w", err)
	}

	return groupMsgs, nil
}

const maxTimeRange = 31 * 24 * time.Hour // Maximum 31 days range

// validateTimeRange ensures the time range is valid.
func validateTimeRange(start, end time.Time) error {
	if start.IsZero() || end.IsZero() {
		return ErrZeroTimeValue
	}

	if start.After(end) {
		return ErrInvalidTimeRange
	}

	if end.Sub(start) > maxTimeRange {
		return fmt.Errorf("%w: %v", ErrTimeRangeExceeded, maxTimeRange)
	}

	return nil
}

// DeleteChatHistory removes only the chat history, preserving user profiles and group messages.
func (d *sqliteDB) DeleteChatHistory() error {
	if err := d.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChatHistory{}).Error; err != nil {
		return fmt.Errorf("%w: failed to delete chat history: %w", ErrDatabaseOperation, err)
	}

	return nil
}

// DeleteAll removes all stored data in a single transaction.
func (d *sqliteDB) DeleteAll() error {
	if err := d.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChatHistory{}).Error; err != nil {
			return fmt.Errorf("failed to delete chat history: %w", err)
		}

		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&GroupMessage{}).Error; err != nil {
			return fmt.Errorf("failed to delete group messages: %w", err)
		}

		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&UserProfile{}).Error; err != nil {
			return fmt.Errorf("failed to delete user profiles: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("%w: %w", ErrDatabaseOperation, err)
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

		return nil, fmt.Errorf("failed to get user profile: %w", result.Error)
	}

	return &profile, nil
}

// SaveUserProfile creates or updates a user profile.
func (d *sqliteDB) SaveUserProfile(profile *UserProfile) error {
	// Check if profile exists
	existingProfile, err := d.GetUserProfile(profile.UserID)
	if err != nil {
		return fmt.Errorf("failed to check existing profile: %w", err)
	}

	// If profile exists, update it
	if existingProfile != nil {
		profile.ID = existingProfile.ID
		profile.CreatedAt = existingProfile.CreatedAt
		profile.LastUpdated = time.Now().UTC()

		if err := d.db.Save(profile).Error; err != nil {
			return fmt.Errorf("failed to update user profile: %w", err)
		}

		return nil
	}

	// Otherwise create a new profile
	profile.LastUpdated = time.Now().UTC()
	if err := d.db.Create(profile).Error; err != nil {
		return fmt.Errorf("failed to create user profile: %w", err)
	}

	return nil
}

// GetAllUserProfiles retrieves all user profiles.
func (d *sqliteDB) GetAllUserProfiles() (map[int64]*UserProfile, error) {
	var profiles []UserProfile
	if err := d.db.Find(&profiles).Error; err != nil {
		return nil, fmt.Errorf("failed to get all user profiles: %w", err)
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
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	return nil
}
