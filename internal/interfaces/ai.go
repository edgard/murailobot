package interfaces

import (
	"context"

	"github.com/edgard/murailobot/internal/models"
)

// AI defines the interface for AI operations
type AI interface {
	// GenerateResponse generates AI response for messages
	GenerateResponse(ctx context.Context, messages []*models.Message) (string, error)

	// GenerateProfile generates user profile from messages
	GenerateProfile(ctx context.Context, userID int64, messages []*models.Message) (*models.UserProfile, error)
}
