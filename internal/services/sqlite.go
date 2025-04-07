package services

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/edgard/murailobot/internal/common"
	"github.com/edgard/murailobot/internal/interfaces"
	"github.com/edgard/murailobot/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SQL implements the DB interface using SQLite
type SQL struct {
	db     *gorm.DB
	dbPath string
}

// NewSQL creates a new SQL service instance
func NewSQL() (interfaces.DB, error) {
	return &SQL{}, nil
}

// Configure sets up the database with the given path
func (s *SQL) Configure(path string) error {
	s.dbPath = path

	// Configure custom logger for GORM
	gormLogger := logger.New(
		&gormLogger{writer: os.Stderr},
		logger.Config{
			LogLevel: logger.Error, // Only log errors
		})

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrDatabaseInit, err)
	}

	// Run migrations
	if err := db.AutoMigrate(&models.Message{}, &models.UserProfile{}); err != nil {
		return fmt.Errorf("%w: %v", common.ErrDatabaseMigration, err)
	}

	s.db = db
	return nil
}

// GetMessages retrieves messages for a group
func (s *SQL) GetMessages(ctx context.Context, groupID int64, limit int, before time.Time) ([]*models.Message, error) {
	var messages []*models.Message

	result := s.db.WithContext(ctx).
		Where("group_id = ? AND created_at < ?", groupID, before).
		Order("created_at desc").
		Limit(limit).
		Find(&messages)

	if result.Error != nil {
		return nil, fmt.Errorf("%w: %v", common.ErrMessageFetch, result.Error)
	}

	return messages, nil
}

// GetUnprocessedMessages retrieves unprocessed messages
func (s *SQL) GetUnprocessedMessages(ctx context.Context) ([]*models.Message, error) {
	var messages []*models.Message

	result := s.db.WithContext(ctx).
		Where("processed = ?", false).
		Order("created_at desc").
		Find(&messages)

	if result.Error != nil {
		return nil, fmt.Errorf("%w: %v", common.ErrMessageFetch, result.Error)
	}

	return messages, nil
}

// SaveMessage saves a message to the database
func (s *SQL) SaveMessage(ctx context.Context, msg *models.Message) error {
	result := s.db.WithContext(ctx).Create(msg)
	if result.Error != nil {
		return fmt.Errorf("%w: %v", common.ErrMessageSave, result.Error)
	}
	return nil
}

// MarkMessagesAsProcessed marks messages as processed
func (s *SQL) MarkMessagesAsProcessed(ctx context.Context, messageIDs []uint) error {
	result := s.db.WithContext(ctx).
		Model(&models.Message{}).
		Where("id IN ?", messageIDs).
		Update("processed", true)

	if result.Error != nil {
		return fmt.Errorf("%w: %v", common.ErrDatabaseOp, result.Error)
	}

	return nil
}

// DeleteMessages deletes messages for a group
func (s *SQL) DeleteMessages(ctx context.Context, groupID int64) error {
	result := s.db.WithContext(ctx).
		Where("group_id = ?", groupID).
		Delete(&models.Message{})

	if result.Error != nil {
		return fmt.Errorf("%w: %v", common.ErrMessageDelete, result.Error)
	}

	return nil
}

// GetProfile retrieves a user profile
func (s *SQL) GetProfile(ctx context.Context, userID int64) (*models.UserProfile, error) {
	var profile models.UserProfile
	result := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		First(&profile)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %v", common.ErrProfileFetch, result.Error)
	}

	return &profile, nil
}

// GetAllProfiles retrieves all user profiles
func (s *SQL) GetAllProfiles(ctx context.Context) (map[int64]*models.UserProfile, error) {
	var profiles []*models.UserProfile
	result := s.db.WithContext(ctx).Find(&profiles)
	if result.Error != nil {
		return nil, fmt.Errorf("%w: %v", common.ErrProfileFetch, result.Error)
	}

	// Convert to map for easier lookup
	profileMap := make(map[int64]*models.UserProfile, len(profiles))
	for _, p := range profiles {
		profileMap[p.UserID] = p
	}

	return profileMap, nil
}

// SaveProfile saves a user profile
func (s *SQL) SaveProfile(ctx context.Context, profile *models.UserProfile) error {
	if profile.ID == 0 {
		profile.CreatedAt = time.Now().UTC()
	}
	profile.LastUpdated = time.Now().UTC()

	result := s.db.WithContext(ctx).Save(profile)
	if result.Error != nil {
		return fmt.Errorf("%w: %v", common.ErrProfileSave, result.Error)
	}

	return nil
}

// BatchSaveProfiles saves multiple user profiles in a single transaction
func (s *SQL) BatchSaveProfiles(ctx context.Context, profiles []*models.UserProfile) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()

		for _, profile := range profiles {
			if profile.ID == 0 {
				profile.CreatedAt = now
			}
			profile.LastUpdated = now

			if err := tx.Save(profile).Error; err != nil {
				return fmt.Errorf("%w: user_id %d: %v", common.ErrProfileSave, profile.UserID, err)
			}
		}

		return nil
	})
}

// Stop closes the database connection and releases any resources
func (s *SQL) Stop() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrDatabaseOp, err)
	}

	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("%w: %v", common.ErrDatabaseOp, err)
	}

	return nil
}

// gormLogger implements a custom logger for GORM that uses slog
type gormLogger struct {
	writer io.Writer
}

func (l *gormLogger) Printf(format string, args ...interface{}) {
	fmt.Fprintf(l.writer, format+"\n", args...)
}
