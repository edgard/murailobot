// Package sqlite provides a SQLite implementation of the store port.
package sqlite

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/edgard/murailobot/internal/common/config"
	"github.com/edgard/murailobot/internal/domain/model"
	"github.com/edgard/murailobot/internal/port/store"
)

// DB models for GORM
type Message struct {
	ID          uint `gorm:"primarykey"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time `gorm:"index"`
	GroupID     int64      `gorm:"not null;index"`
	GroupName   string     `gorm:"type:text"`
	UserID      int64      `gorm:"not null;index"`
	Content     string     `gorm:"not null;type:text"`
	Timestamp   time.Time  `gorm:"not null;index"`
	ProcessedAt *time.Time `gorm:"index"`
}

type UserProfile struct {
	ID              uint `gorm:"primarykey"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time `gorm:"index"`
	UserID          int64      `gorm:"not null;uniqueIndex"`
	DisplayNames    string     `gorm:"type:text"`
	OriginLocation  string     `gorm:"type:text"`
	CurrentLocation string     `gorm:"type:text"`
	AgeRange        string     `gorm:"type:text"`
	Traits          string     `gorm:"type:text"`
	LastUpdated     time.Time  `gorm:"not null"`
}

// dbStore provides a SQLite implementation of the store.Store interface
type dbStore struct {
	db     *gorm.DB
	logger *zap.Logger
}

// gormLogAdapter is an adapter to use zap with gorm's logger interface
type gormLogAdapter struct {
	logger *zap.Logger
	config gormlogger.Config
}

// newGormLogAdapter creates a new logger adapter for GORM
func newGormLogAdapter(logger *zap.Logger, config gormlogger.Config) gormlogger.Interface {
	return &gormLogAdapter{
		logger: logger,
		config: config,
	}
}

// LogMode implements gormlogger.Interface
func (l *gormLogAdapter) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	newLogger := *l
	newLogger.config.LogLevel = level
	return &newLogger
}

// Info implements gormlogger.Interface
func (l *gormLogAdapter) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.config.LogLevel >= gormlogger.Info {
		l.logger.Sugar().Infof(msg, data...)
	}
}

// Warn implements gormlogger.Interface
func (l *gormLogAdapter) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.config.LogLevel >= gormlogger.Warn {
		l.logger.Sugar().Warnf(msg, data...)
	}
}

// Error implements gormlogger.Interface
func (l *gormLogAdapter) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.config.LogLevel >= gormlogger.Error {
		l.logger.Sugar().Errorf(msg, data...)
	}
}

// Trace implements gormlogger.Interface
func (l *gormLogAdapter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.config.LogLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	switch {
	case err != nil && l.config.LogLevel >= gormlogger.Error:
		sql, rows := fc()
		l.logger.Error("gorm error",
			zap.Error(err),
			zap.Duration("elapsed", elapsed),
			zap.String("sql", sql),
			zap.Int64("rows", rows),
		)
	case elapsed > l.config.SlowThreshold && l.config.SlowThreshold != 0 && l.config.LogLevel >= gormlogger.Warn:
		sql, rows := fc()
		l.logger.Warn("gorm slow query",
			zap.Duration("elapsed", elapsed),
			zap.String("sql", sql),
			zap.Int64("rows", rows),
			zap.Duration("threshold", l.config.SlowThreshold),
		)
	case l.config.LogLevel >= gormlogger.Info:
		sql, rows := fc()
		l.logger.Debug("gorm query",
			zap.Duration("elapsed", elapsed),
			zap.String("sql", sql),
			zap.Int64("rows", rows),
		)
	}
}

// NewStore creates a new SQLite store with the provided configuration.
func NewStore(cfg *config.Config, logger *zap.Logger) (store.Store, error) {
	startTime := time.Now()

	logger.Debug("initializing database", zap.String("path", cfg.DBPath))

	// Configure GORM logger
	gormLoggerConfig := gormlogger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  gormlogger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	}

	gormLogger := newGormLogAdapter(logger.Named("gorm"), gormLoggerConfig)

	gormConfig := &gorm.Config{
		Logger: gormLogger,
	}

	// Open database connection and configure pool
	dbOpenStart := time.Now()
	db, err := gorm.Open(sqlite.Open(cfg.DBPath), gormConfig)
	if err != nil {
		logger.Error("failed to open database",
			zap.Error(err),
			zap.String("path", cfg.DBPath),
			zap.Int64("duration_ms", time.Since(dbOpenStart).Milliseconds()))

		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Get SQL DB instance for connection pool configuration
	sqlDB, err := db.DB()
	if err != nil {
		logger.Error("failed to get database instance", zap.Error(err))

		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(1)                // Single connection for SQLite
	sqlDB.SetMaxIdleConns(1)                // Keep connection idle in pool
	sqlDB.SetConnMaxLifetime(1 * time.Hour) // Recycle after 1 hour

	logger.Debug("database connection configured",
		zap.String("path", cfg.DBPath),
		zap.Int64("duration_ms", time.Since(dbOpenStart).Milliseconds()))

	// Run migrations
	migrationStart := time.Now()
	if err := db.AutoMigrate(&Message{}, &UserProfile{}); err != nil {
		logger.Error("failed to run migrations",
			zap.Error(err),
			zap.Int64("duration_ms", time.Since(migrationStart).Milliseconds()))

		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	totalDuration := time.Since(startTime)
	logger.Info("database initialization complete",
		zap.Int64("duration_ms", totalDuration.Milliseconds()),
		zap.Int64("migration_ms", time.Since(migrationStart).Milliseconds()))

	return &dbStore{
		db:     db,
		logger: logger,
	}, nil
}

// Convert between domain model and DB model
func toDBMessage(m *model.Message) *Message {
	return &Message{
		ID:          m.ID,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
		DeletedAt:   m.DeletedAt,
		GroupID:     m.GroupID,
		GroupName:   m.GroupName,
		UserID:      m.UserID,
		Content:     m.Content,
		Timestamp:   m.Timestamp,
		ProcessedAt: m.ProcessedAt,
	}
}

func toDomainMessage(m *Message) *model.Message {
	return &model.Message{
		ID:          m.ID,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
		DeletedAt:   m.DeletedAt,
		GroupID:     m.GroupID,
		GroupName:   m.GroupName,
		UserID:      m.UserID,
		Content:     m.Content,
		Timestamp:   m.Timestamp,
		ProcessedAt: m.ProcessedAt,
	}
}

func toDBUserProfile(p *model.UserProfile) *UserProfile {
	return &UserProfile{
		ID:              p.ID,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
		DeletedAt:       p.DeletedAt,
		UserID:          p.UserID,
		DisplayNames:    p.DisplayNames,
		OriginLocation:  p.OriginLocation,
		CurrentLocation: p.CurrentLocation,
		AgeRange:        p.AgeRange,
		Traits:          p.Traits,
		LastUpdated:     p.LastUpdated,
	}
}

func toDomainUserProfile(p *UserProfile) *model.UserProfile {
	return &model.UserProfile{
		ID:              p.ID,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
		DeletedAt:       p.DeletedAt,
		UserID:          p.UserID,
		DisplayNames:    p.DisplayNames,
		OriginLocation:  p.OriginLocation,
		CurrentLocation: p.CurrentLocation,
		AgeRange:        p.AgeRange,
		Traits:          p.Traits,
		LastUpdated:     p.LastUpdated,
	}
}

// Implementation of store.Store interface
func (s *dbStore) SaveMessage(ctx context.Context, msg *model.Message) error {
	if msg == nil {
		return errors.New("nil message")
	}

	// Only measure timing if we're going to log it (slow operation)
	startTime := time.Now()

	dbMsg := toDBMessage(msg)
	err := s.db.WithContext(ctx).Create(dbMsg).Error
	if err != nil {
		return fmt.Errorf("failed to save message for user %d: %w", msg.UserID, err)
	}

	// Update domain model with generated ID
	msg.ID = dbMsg.ID
	msg.CreatedAt = dbMsg.CreatedAt
	msg.UpdatedAt = dbMsg.UpdatedAt

	// Only log slow operations to reduce noise
	slowThreshold := 100 * time.Millisecond
	duration := time.Since(startTime)

	if duration > slowThreshold {
		s.logger.Warn("slow database operation detected",
			zap.String("operation", "save_message"),
			zap.Int64("group_id", msg.GroupID),
			zap.Int64("duration_ms", duration.Milliseconds()))
	}

	return nil
}

func (s *dbStore) GetRecentMessages(ctx context.Context, groupID int64, limit int, beforeTimestamp time.Time, beforeID uint) ([]*model.Message, error) {
	if limit <= 0 {
		return nil, errors.New("invalid limit")
	}

	query := s.db.WithContext(ctx).
		Where("group_id = ?", groupID).
		Order("timestamp desc, id desc").
		Limit(limit)

	// Apply timestamp and ID filters if provided
	if !beforeTimestamp.IsZero() {
		if beforeID > 0 {
			// When both timestamp and ID are provided
			query = query.Where("(timestamp < ?) OR (timestamp = ? AND id < ?)",
				beforeTimestamp, beforeTimestamp, beforeID)
		} else {
			// When only timestamp is provided
			query = query.Where("timestamp <= ?", beforeTimestamp)
		}
	} else if beforeID > 0 {
		// When only ID is provided
		query = query.Where("id < ?", beforeID)
	}

	// Query the database
	var dbMessages []*Message
	if err := query.Find(&dbMessages).Error; err != nil {
		return nil, fmt.Errorf("failed to get recent messages: %w", err)
	}

	// Convert to domain models
	messages := make([]*model.Message, len(dbMessages))
	for i, dbMsg := range dbMessages {
		messages[i] = toDomainMessage(dbMsg)
	}

	// Sort messages chronologically
	sort.Slice(messages, func(i, j int) bool {
		// First sort by timestamp
		if !messages[i].Timestamp.Equal(messages[j].Timestamp) {
			return messages[i].Timestamp.Before(messages[j].Timestamp)
		}
		// If timestamps are equal, sort by ID for consistent ordering
		return messages[i].ID < messages[j].ID
	})

	// Only log if an unusual number of messages is retrieved
	if len(messages) == 0 || len(messages) == limit {
		s.logger.Debug("messages retrieved",
			zap.Int64("group_id", groupID),
			zap.Int("count", len(messages)),
			zap.Time("before_timestamp", beforeTimestamp),
			zap.Uint("before_id", beforeID))
	}

	return messages, nil
}

func (s *dbStore) GetUnprocessedMessages(ctx context.Context) ([]*model.Message, error) {
	var dbMessages []*Message

	if err := s.db.WithContext(ctx).
		Where("processed_at IS NULL").
		Order("timestamp asc").
		Find(&dbMessages).Error; err != nil {
		return nil, fmt.Errorf("failed to get unprocessed messages: %w", err)
	}

	// Convert to domain models
	messages := make([]*model.Message, len(dbMessages))
	for i, dbMsg := range dbMessages {
		messages[i] = toDomainMessage(dbMsg)
	}

	return messages, nil
}

func (s *dbStore) MarkMessagesAsProcessed(ctx context.Context, messageIDs []uint) error {
	if len(messageIDs) == 0 {
		return errors.New("empty message IDs")
	}

	now := time.Now().UTC()

	batchSize := 100
	for i := 0; i < len(messageIDs); i += batchSize {
		end := i + batchSize
		if end > len(messageIDs) {
			end = len(messageIDs)
		}

		batch := messageIDs[i:end]
		if err := s.db.WithContext(ctx).
			Model(&Message{}).
			Where("id IN ?", batch).
			Update("processed_at", now).Error; err != nil {
			return fmt.Errorf("failed to mark messages as processed: %w", err)
		}
	}

	return nil
}

func (s *dbStore) GetUserProfile(ctx context.Context, userID int64) (*model.UserProfile, error) {
	var dbProfile UserProfile
	result := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&dbProfile)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get user profile: %w", result.Error)
	}

	return toDomainUserProfile(&dbProfile), nil
}

func (s *dbStore) SaveUserProfile(ctx context.Context, profile *model.UserProfile) error {
	if profile == nil {
		return errors.New("nil profile")
	}

	if profile.UserID <= 0 {
		return errors.New("invalid user ID")
	}

	// Set last updated timestamp
	profile.LastUpdated = time.Now().UTC()

	// Convert to DB model
	dbProfile := toDBUserProfile(profile)

	// Check if profile already exists
	var existingProfile UserProfile
	result := s.db.WithContext(ctx).Where("user_id = ?", profile.UserID).First(&existingProfile)

	var err error
	var isNew bool

	// Update or create based on existence
	if result.Error == nil {
		// Update existing profile - preserve metadata
		dbProfile.ID = existingProfile.ID
		dbProfile.CreatedAt = existingProfile.CreatedAt
		err = s.db.WithContext(ctx).Save(dbProfile).Error
	} else if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// Create new profile
		isNew = true
		err = s.db.WithContext(ctx).Create(dbProfile).Error
	} else {
		// Unexpected error
		return fmt.Errorf("failed to check existing profile: %w", result.Error)
	}

	if err != nil {
		return fmt.Errorf("failed to save user profile: %w", err)
	}

	// Update domain model with database-assigned values
	profile.ID = dbProfile.ID
	profile.CreatedAt = dbProfile.CreatedAt
	profile.UpdatedAt = dbProfile.UpdatedAt

	// Only log at Info level for new profiles, Debug for updates
	if isNew {
		s.logger.Info("new user profile created", zap.Int64("user_id", profile.UserID))
	} else {
		s.logger.Debug("user profile updated", zap.Int64("user_id", profile.UserID))
	}

	return nil
}

func (s *dbStore) GetAllUserProfiles(ctx context.Context) (map[int64]*model.UserProfile, error) {
	var dbProfiles []*UserProfile
	if err := s.db.WithContext(ctx).Find(&dbProfiles).Error; err != nil {
		return nil, fmt.Errorf("failed to get all user profiles: %w", err)
	}

	result := make(map[int64]*model.UserProfile, len(dbProfiles))
	for _, dbProfile := range dbProfiles {
		result[dbProfile.UserID] = toDomainUserProfile(dbProfile)
	}

	return result, nil
}

func (s *dbStore) DeleteAll(ctx context.Context) error {
	if err := s.db.WithContext(ctx).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Message{}).Error; err != nil {
		return fmt.Errorf("failed to delete all messages: %w", err)
	}

	if err := s.db.WithContext(ctx).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&UserProfile{}).Error; err != nil {
		return fmt.Errorf("failed to delete all user profiles: %w", err)
	}

	return nil
}

func (s *dbStore) Close() error {
	// Get database connection
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	// Get connection stats before closing
	stats := sqlDB.Stats()

	// Only log warnings if there are issues with the connection pool
	if stats.OpenConnections > 5 || float64(stats.InUse)/float64(stats.OpenConnections+1) > 0.8 {
		s.logger.Warn("database connection pool pressure detected",
			zap.Int("open_connections", stats.OpenConnections),
			zap.Int("in_use", stats.InUse),
			zap.Float64("utilization_percent", float64(stats.InUse)/float64(stats.OpenConnections+1)*100))
	}

	// Close the connection
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	s.logger.Debug("database connection closed")

	return nil
}
