package db

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// New creates a new SQLite database connection with optimized settings.
// If no configuration is provided, it uses default values for chat history storage.
func New(cfg *Config) (*SQLiteDB, error) {
	if cfg == nil {
		cfg = &Config{
			TempStore:   defaultTempStore,
			CacheSizeKB: defaultCacheSizeKB,
			OpTimeout:   defaultOpTimeout,
		}
	}

	gormCfg := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	// Configure SQLite with optimized settings:
	// - WAL journal mode for better concurrency
	// - Busy timeout to handle concurrent access
	// - Memory-based temp store for better performance
	// - Configured page cache size
	dsn := "storage.db?_journal=WAL" +
		"&_timeout=" + strconv.Itoa(defaultDSNTimeout) +
		"&_temp_store=" + cfg.TempStore +
		"&_cache_size=-" + strconv.Itoa(cfg.CacheSizeKB)

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
		db:  db,
		cfg: cfg,
	}, nil
}

// withTimeout creates a timeout context based on the provided context.
// Returns the original context if it already has an appropriate deadline,
// otherwise creates a new context with the configured timeout.
func (d *SQLiteDB) withTimeout(parentCtx context.Context) (context.Context, context.CancelFunc) {
	deadline, ok := parentCtx.Deadline()
	if !ok || time.Until(deadline) > d.cfg.OpTimeout {
		// No deadline or deadline is too far in the future, create a timeout context
		return context.WithTimeout(parentCtx, d.cfg.OpTimeout)
	}
	// Context already has an appropriate deadline
	return parentCtx, func() {} // Noop cancel function
}

// GetRecent retrieves the most recent chat history entries, ordered by timestamp.
// Enforces a timeout through context to prevent long-running queries.
func (d *SQLiteDB) GetRecent(parentCtx context.Context, limit int) ([]ChatHistory, error) {
	select {
	case <-parentCtx.Done():
		return nil, fmt.Errorf("context canceled before retrieving history: %w", parentCtx.Err())
	default:
	}

	var history []ChatHistory

	opCtx, opCancel := d.withTimeout(parentCtx)
	defer opCancel()

	if err := d.db.WithContext(opCtx).
		Order("timestamp desc").
		Limit(limit).
		Find(&history).Error; err != nil {
		return nil, fmt.Errorf("failed to get recent history: %w", err)
	}

	return history, nil
}

// Save stores a new chat interaction in the database with the current UTC timestamp.
func (d *SQLiteDB) Save(parentCtx context.Context, userID int64, userName string, userMsg, botMsg string) error {
	select {
	case <-parentCtx.Done():
		return fmt.Errorf("context canceled before saving chat history: %w", parentCtx.Err())
	default:
	}

	history := ChatHistory{
		UserID:    userID,
		UserName:  userName,
		UserMsg:   userMsg,
		BotMsg:    botMsg,
		Timestamp: time.Now().UTC(),
	}

	opCtx, opCancel := d.withTimeout(parentCtx)
	defer opCancel()

	if err := d.db.WithContext(opCtx).Create(&history).Error; err != nil {
		return fmt.Errorf("failed to save chat history: %w", err)
	}

	return nil
}

// DeleteAll removes all chat history entries from the database.
// This administrative operation enforces a timeout to prevent long-running operations.
func (d *SQLiteDB) DeleteAll(parentCtx context.Context) error {
	select {
	case <-parentCtx.Done():
		return fmt.Errorf("context canceled before deleting all history: %w", parentCtx.Err())
	default:
	}

	opCtx, opCancel := d.withTimeout(parentCtx)
	defer opCancel()

	if err := d.db.WithContext(opCtx).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChatHistory{}).Error; err != nil {
		return fmt.Errorf("failed to delete all history: %w", err)
	}

	return nil
}

// Close properly closes the database connection, ensuring all pending
// operations are completed and resources are released.
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
