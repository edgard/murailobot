package db

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func New(cfg *Config) (Database, error) {
	if cfg == nil {
		cfg = &Config{
			TempStore:   "MEMORY",
			CacheSizeKB: 4000,
			OpTimeout:   15 * time.Second,
		}
	}

	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	dsn := "storage.db?_journal=WAL&_timeout=5000&_temp_store=" + cfg.TempStore +
		"&_cache_size=-" + fmt.Sprintf("%d", cfg.CacheSizeKB)

	db, err := gorm.Open(sqlite.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(&ChatHistory{}); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &database{
		db:  db,
		cfg: cfg,
	}, nil
}

func (d *database) Save(ctx context.Context, userID int64, userName string, userMsg, botMsg string) error {
	history := ChatHistory{
		UserID:    userID,
		UserName:  userName,
		UserMsg:   userMsg,
		BotMsg:    botMsg,
		Timestamp: time.Now().UTC(),
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, d.cfg.OpTimeout)
	defer cancel()

	if err := d.db.WithContext(timeoutCtx).Create(&history).Error; err != nil {
		return fmt.Errorf("failed to save chat history: %w", err)
	}

	return nil
}

func (d *database) GetRecent(ctx context.Context, limit int) ([]ChatHistory, error) {
	var history []ChatHistory
	timeoutCtx, cancel := context.WithTimeout(ctx, d.cfg.OpTimeout)
	defer cancel()

	if err := d.db.WithContext(timeoutCtx).
		Order("timestamp desc").
		Limit(limit).
		Find(&history).Error; err != nil {
		return nil, fmt.Errorf("failed to get recent history: %w", err)
	}

	return history, nil
}

func (d *database) DeleteAll(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, d.cfg.OpTimeout)
	defer cancel()

	if err := d.db.WithContext(timeoutCtx).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChatHistory{}).Error; err != nil {
		return fmt.Errorf("failed to delete all history: %w", err)
	}

	return nil
}

func (d *database) Close() error {
	sqlDB, err := d.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	return nil
}
