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

// GetRecent retrieves the most recent chat history entries, ordered by timestamp.
func (d *SQLiteDB) GetRecent(limit int) ([]ChatHistory, error) {
	var history []ChatHistory

	if err := d.db.Order("timestamp desc").
		Limit(limit).
		Find(&history).Error; err != nil {
		return nil, fmt.Errorf("failed to get recent history: %w", err)
	}

	return history, nil
}

// Save stores a new chat interaction with the current UTC timestamp.
func (d *SQLiteDB) Save(userID int64, userName string, userMsg, botMsg string) error {
	history := ChatHistory{
		UserID:    userID,
		UserName:  userName,
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
func (d *SQLiteDB) SaveGroupMessage(groupID int64, groupName string, userID int64, userName string, message string) error {
	groupMsg := GroupMessage{
		GroupID:   groupID,
		GroupName: groupName,
		UserID:    userID,
		UserName:  userName,
		Message:   message,
		Timestamp: time.Now().UTC(),
	}

	if err := d.db.Create(&groupMsg).Error; err != nil {
		return fmt.Errorf("failed to save group message: %w", err)
	}

	return nil
}

// GetMessagesByUserInTimeRange retrieves all messages from a user within a time range.
func (d *SQLiteDB) GetMessagesByUserInTimeRange(userID int64, start, end time.Time) ([]GroupMessage, error) {
	var messages []GroupMessage

	if err := d.db.Where("user_id = ? AND timestamp >= ? AND timestamp < ?", userID, start, end).
		Order("timestamp asc").
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get user messages: %w", err)
	}

	return messages, nil
}

// GetActiveUsersInTimeRange retrieves all users who sent messages in the time range.
func (d *SQLiteDB) GetActiveUsersInTimeRange(start, end time.Time) (map[int64]string, error) {
	var messages []GroupMessage

	users := make(map[int64]string)

	if err := d.db.Select("DISTINCT user_id, user_name").
		Where("timestamp >= ? AND timestamp < ?", start, end).
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get active users: %w", err)
	}

	for _, msg := range messages {
		users[msg.UserID] = msg.UserName
	}

	return users, nil
}

// SaveUserAnalysis stores personality/behavioral analysis for a user.
func (d *SQLiteDB) SaveUserAnalysis(analysis *UserAnalysis) error {
	if err := d.db.Create(analysis).Error; err != nil {
		return fmt.Errorf("failed to save user analysis: %w", err)
	}

	return nil
}

// GetUserAnalysesByDateRange retrieves user analyses within a date range.
func (d *SQLiteDB) GetUserAnalysesByDateRange(start, end time.Time) ([]UserAnalysis, error) {
	var analyses []UserAnalysis

	if err := d.db.Where("date >= ? AND date < ?", start, end).
		Order("date asc, user_id asc").
		Find(&analyses).Error; err != nil {
		return nil, fmt.Errorf("failed to get user analyses: %w", err)
	}

	return analyses, nil
}

// DeleteAll removes all chat history entries.
func (d *SQLiteDB) DeleteAll() error {
	if err := d.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChatHistory{}).Error; err != nil {
		return fmt.Errorf("failed to delete chat history: %w", err)
	}

	if err := d.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&GroupMessage{}).Error; err != nil {
		return fmt.Errorf("failed to delete group messages: %w", err)
	}

	if err := d.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&UserAnalysis{}).Error; err != nil {
		return fmt.Errorf("failed to delete user analyses: %w", err)
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
