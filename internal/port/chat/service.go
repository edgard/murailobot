// Package chat defines the interface for chat service interactions.
package chat

import (
	"context"
)

// Service defines the interface for chat operations.
type Service interface {
	// Start begins processing incoming chat updates.
	Start() error

	// Stop gracefully shuts down the chat service.
	Stop() error

	// SendUserProfiles formats and sends all user profiles to the specified chat.
	SendUserProfiles(ctx context.Context, chatID int64) error

	// IsAuthorized checks if a user is authorized for admin actions.
	IsAuthorized(userID int64) bool
}
