package interfaces

import (
	"context"
	"time"

	"github.com/edgard/murailobot/internal/models"
)

// AI defines the interface for AI operations
type AI interface {
	// Configure sets up the AI service
	Configure(token, baseURL, model string, maxTokens int, temperature float32,
		timeout time.Duration, instruction, profileInstruction string) error

	// GenerateResponse generates an AI response for messages
	GenerateResponse(ctx context.Context, messages []*models.Message, botInfo models.BotInfo) (string, error)

	// GenerateProfile generates a user profile from messages
	GenerateProfile(ctx context.Context, userID int64, messages []*models.Message, botInfo models.BotInfo) (*models.UserProfile, error)

	// Stop gracefully shuts down the AI service
	Stop() error
}
