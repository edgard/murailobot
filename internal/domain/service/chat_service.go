// Package service contains the core domain services for MurailoBot
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/edgard/murailobot/internal/common/config"
	"github.com/edgard/murailobot/internal/port/ai"
	"github.com/edgard/murailobot/internal/port/chat"
	"github.com/edgard/murailobot/internal/port/scheduler"
	"github.com/edgard/murailobot/internal/port/store"
)

// ChatService is the main service orchestrating the application's components.
type ChatService struct {
	config      *config.Config
	store       store.Store
	aiService   ai.Service
	chatService chat.Service
	scheduler   scheduler.Service
}

// NewChatService creates a new chat service with the required dependencies.
func NewChatService(
	cfg *config.Config,
	store store.Store,
	aiService ai.Service,
	chatService chat.Service,
	scheduler scheduler.Service,
) *ChatService {
	return &ChatService{
		config:      cfg,
		store:       store,
		aiService:   aiService,
		chatService: chatService,
		scheduler:   scheduler,
	}
}

// Start initializes the service and starts processing updates.
func (s *ChatService) Start(errCh chan<- error) error {
	slog.Debug("starting chat service")
	return s.chatService.Start(errCh)
}

// Stop gracefully shuts down the service and releases resources.
func (s *ChatService) Stop() error {
	slog.Info("stopping chat service")

	// Define a context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Log shutdown sequence
	slog.Debug("stopping chat adapter")
	if err := s.chatService.Stop(); err != nil {
		slog.Error("error stopping chat adapter", "error", err)
	}

	slog.Debug("stopping scheduler")
	if err := s.scheduler.Stop(); err != nil {
		slog.Error("error stopping scheduler", "error", err)
	}

	// Close the store connection last
	slog.Debug("closing store connection")
	if err := s.store.Close(); err != nil {
		slog.Error("error closing store connection", "error", err)
		return err
	}

	slog.Info("chat service stopped successfully")

	// Check if the context has expired
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}
