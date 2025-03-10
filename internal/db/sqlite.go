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

	if err := db.AutoMigrate(&ChatHistory{}, &GroupMessage{}, &UserAnalysis{}); err != nil {
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

// SaveUserAnalysis stores personality/behavioral analysis for a user.
func (d *sqliteDB) SaveUserAnalysis(analysis *UserAnalysis) error {
	if err := d.db.Create(analysis).Error; err != nil {
		return fmt.Errorf("failed to save user analysis: %w", err)
	}

	return nil
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

// GetUserAnalysesInTimeRange retrieves user analyses within a time range.
func (d *sqliteDB) GetUserAnalysesInTimeRange(start, end time.Time) ([]UserAnalysis, error) {
	if err := validateTimeRange(start, end); err != nil {
		return nil, fmt.Errorf("invalid time range: %w", err)
	}

	var userAnalyses []UserAnalysis
	if err := d.db.Where("date >= ? AND date < ?", start, end).
		Order("date asc, user_id asc").
		Find(&userAnalyses).Error; err != nil {
		return nil, fmt.Errorf("failed to get user analyses: %w", err)
	}

	return userAnalyses, nil
}

// DeleteAll removes all chat history entries in a single transaction.
func (d *sqliteDB) DeleteAll() error {
	if err := d.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChatHistory{}).Error; err != nil {
			return fmt.Errorf("failed to delete chat history: %w", err)
		}

		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&GroupMessage{}).Error; err != nil {
			return fmt.Errorf("failed to delete group messages: %w", err)
		}

		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&UserAnalysis{}).Error; err != nil {
			return fmt.Errorf("failed to delete user analyses: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("%w: %w", ErrDatabaseOperation, err)
	}

	return nil
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
