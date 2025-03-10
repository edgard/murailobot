package db

import (
	"fmt"
	"strconv"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// New creates a SQLite database connection optimized for chat history storage.
// It configures WAL journaling, busy timeout, and memory-based temp storage.
func New() (*SQLiteDB, error) {
	gormCfg := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	dsn := "storage.db?_journal=WAL" +
		"&_timeout=" + strconv.Itoa(defaultDSNTimeout) +
		"&_temp_store=" + defaultTempStore +
		"&_cache_size=-" + strconv.Itoa(defaultCacheSizeKB)

	db, err := gorm.Open(sqlite.Open(dsn), gormCfg)
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

	return &SQLiteDB{
		db: db,
	}, nil
}

// GetRecent retrieves the most recent chat history entries.
func (d *SQLiteDB) GetRecent(limit int) ([]ChatHistory, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidLimit, limit)
	}

	var history []ChatHistory
	if err := d.db.Order("timestamp desc").
		Limit(limit).
		Find(&history).Error; err != nil {
		return nil, fmt.Errorf("failed to get recent history: %w", err)
	}

	return history, nil
}

// Save stores a new chat interaction with the current UTC timestamp.
func (d *SQLiteDB) Save(userID int64, userMsg, botMsg string) error {
	history := ChatHistory{
		UserID:    userID,
		UserMsg:   userMsg,
		BotMsg:    botMsg,
		Timestamp: time.Now().UTC(),
	}

	if err := d.db.Create(&history).Error; err != nil {
		return fmt.Errorf("failed to save chat history: %w", err)
	}

	return nil
}

// SaveGroupMessage stores a message from a group chat.
func (d *SQLiteDB) SaveGroupMessage(groupID int64, groupName string, userID int64, message string) error {
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
func (d *SQLiteDB) GetGroupMessagesInTimeRange(start, end time.Time) ([]GroupMessage, error) {
	if err := validateTimeRange(start, end); err != nil {
		return nil, fmt.Errorf("invalid time range: %w", err)
	}

	var messages []GroupMessage
	if err := d.db.Where("timestamp >= ? AND timestamp < ?", start, end).
		Order("timestamp asc").
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get group messages: %w", err)
	}

	return messages, nil
}

// SaveUserAnalysis stores personality/behavioral analysis for a user.
func (d *SQLiteDB) SaveUserAnalysis(analysis *UserAnalysis) error {
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
func (d *SQLiteDB) GetUserAnalysesInTimeRange(start, end time.Time) ([]UserAnalysis, error) {
	if err := validateTimeRange(start, end); err != nil {
		return nil, fmt.Errorf("invalid time range: %w", err)
	}

	var analyses []UserAnalysis
	if err := d.db.Where("date >= ? AND date < ?", start, end).
		Order("date asc, user_id asc").
		Find(&analyses).Error; err != nil {
		return nil, fmt.Errorf("failed to get user analyses: %w", err)
	}

	return analyses, nil
}

// DeleteAll removes all chat history entries in a single transaction.
func (d *SQLiteDB) DeleteAll() error {
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
func (d *SQLiteDB) Close() error {
	sqlDB, err := d.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	return nil
}
