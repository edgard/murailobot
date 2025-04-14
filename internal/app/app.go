// Package app provides application bootstrapping for MurailoBot.
package app

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/fx"

	"github.com/edgard/murailobot/internal/domain/service"
	fxmodules "github.com/edgard/murailobot/internal/fx"
)

// Application holds the core application components and dependencies.
type Application struct {
	fxApp       *fx.App
	chatService *service.ChatService
	errCh       chan error
	quit        chan os.Signal
}

// New creates a new application instance with all dependencies wired.
func New() (*Application, error) {
	// Create error channel for runtime errors
	errCh := make(chan error, 1)

	// Create signal channel for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Store for our extracted dependencies
	var chatService *service.ChatService

	// Create the fx application with all modules
	fxApp := fx.New(
		fxmodules.RootModule,
		fx.Populate(&chatService),
	)

	// Start the fx app to initialize dependencies
	if err := fxApp.Start(context.Background()); err != nil {
		return nil, err
	}

	return &Application{
		fxApp:       fxApp,
		chatService: chatService,
		errCh:       errCh,
		quit:        quit,
	}, nil
}

// Run starts the application and blocks until shutdown.
// It handles signals and errors properly for graceful shutdown.
func (a *Application) Run() error {
	slog.Info("starting MurailoBot application")

	// Start the chat service in a goroutine
	go func() {
		if err := a.chatService.Start(a.errCh); err != nil {
			slog.Error("failed to start chat service", "error", err)
			a.errCh <- err
		}
	}()

	// Wait for shutdown signal or error
	var err error
	select {
	case sig := <-a.quit:
		slog.Info("received shutdown signal", "signal", sig)
	case receivedErr := <-a.errCh:
		slog.Error("application error", "error", receivedErr)
		err = receivedErr
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Info("shutting down application")
	if stopErr := a.Stop(ctx); stopErr != nil {
		slog.Error("error during shutdown", "error", stopErr)
		if err == nil {
			err = stopErr
		}
	}

	return err
}

// Stop gracefully shuts down the application and releases resources.
func (a *Application) Stop(ctx context.Context) error {
	return a.fxApp.Stop(ctx)
}
