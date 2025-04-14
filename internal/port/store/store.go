// Package store defines the storage port (interface) for the application.
// This port defines how the application interacts with persistent storage.
package store

import (
	"context"
	"time"

	"github.com/edgard/murailobot/internal/domain/model"
)

// Store defines the interface for database operations.
// It provides methods for accessing and manipulating messages and user profiles.
type Store interface {
	// Message operations
	SaveMessage(ctx context.Context, message *model.Message) error
	GetRecentMessages(ctx context.Context, groupID int64, limit int, beforeTime time.Time, beforeID uint) ([]*model.Message, error)
	GetUnprocessedMessages(ctx context.Context) ([]*model.Message, error)
	MarkMessagesAsProcessed(ctx context.Context, messageIDs []uint) error
	DeleteAll(ctx context.Context) error

	// User profile operations
	SaveUserProfile(ctx context.Context, profile *model.UserProfile) error
	GetUserProfile(ctx context.Context, userID int64) (*model.UserProfile, error)
	GetAllUserProfiles(ctx context.Context) (map[int64]*model.UserProfile, error)

	// Lifecycle operations
	Close() error
}
