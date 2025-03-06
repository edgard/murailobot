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

	if err := db.AutoMigrate(&ChatHistory{}); err != nil {
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

// DeleteAll removes all chat history entries.
func (d *SQLiteDB) DeleteAll() error {
	if err := d.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChatHistory{}).Error; err != nil {
		return fmt.Errorf("failed to delete all history: %w", err)
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
