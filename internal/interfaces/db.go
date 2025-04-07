package interfaces

import (
	"context"
	"time"

	"github.com/edgard/murailobot/internal/models"
)

// DB defines the interface for data operations
type DB interface {
	// Message operations
	SaveMessage(ctx context.Context, msg *models.Message) error
	GetMessages(ctx context.Context, groupID int64, limit int, before time.Time) ([]*models.Message, error)
	DeleteMessages(ctx context.Context, groupID int64) error
	GetUnprocessedMessages(ctx context.Context) ([]*models.Message, error)
	MarkMessagesAsProcessed(ctx context.Context, ids []uint) error

	// Profile operations
	SaveProfile(ctx context.Context, profile *models.UserProfile) error
	BatchSaveProfiles(ctx context.Context, profiles []*models.UserProfile) error
	GetProfile(ctx context.Context, userID int64) (*models.UserProfile, error)
	GetAllProfiles(ctx context.Context) (map[int64]*models.UserProfile, error)
}
