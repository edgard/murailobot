// Package telegram handles the setup and registration of Telegram bot handlers.
package telegram

import (
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"

	"github.com/edgard/murailobot/internal/bot/handlers"
)

// NewTelegramBot creates a new Telegram bot instance using the go-telegram/bot library.
func NewTelegramBot(token string, logger *slog.Logger, opts ...bot.Option) (*bot.Bot, error) {
	if token == "" {
		return nil, fmt.Errorf("telegram bot token cannot be empty")
	}
	if logger == nil {
		logger = slog.Default()
	}
	log := logger.With("component", "telegram_bot")

	b, err := bot.New(token, opts...)
	if err != nil {
		log.Error("Failed to create Telegram bot instance", "error", err)
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	log.Info("Telegram bot instance created successfully", "token_prefix", token[:8]+"...")
	return b, nil
}

// applyMiddleware wraps a handler function with a slice of middleware.
// Middleware are applied in reverse order so the first one in the slice is the outermost.
func applyMiddleware(handler bot.HandlerFunc, mw []bot.Middleware) bot.HandlerFunc {
	for i := len(mw) - 1; i >= 0; i-- {
		handler = mw[i](handler)
	}
	return handler
}

// RegisterHandlers registers command and message handlers with the Telegram bot instance.
// It now accepts a map of RegisteredHandler structs and applies middleware.
func RegisterHandlers(b *bot.Bot, logger *slog.Logger, registeredHandlers map[string]handlers.RegisteredHandler) error {
	if b == nil {
		return fmt.Errorf("bot instance cannot be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	log := logger.With("component", "handler_registry")

	if len(registeredHandlers) == 0 {
		log.Warn("No handlers provided for registration.")
		return nil
	}

	log.Info("Registering Telegram handlers...", "count", len(registeredHandlers))

	// Process all handlers in a single pass
	for _, regHandler := range registeredHandlers {
		if regHandler.Handler == nil {
			log.Warn("Skipping registration for nil handler", "pattern", regHandler.Pattern)
			continue
		}

		// Apply middleware associated with this specific handler
		finalHandler := applyMiddleware(regHandler.Handler, regHandler.Middleware)
		// Pattern from registry (empty means catch-all)
		pattern := regHandler.Pattern
		// Register using explicit handler type and match type from RegisteredHandler
		b.RegisterHandler(regHandler.HandlerType, pattern, regHandler.MatchType, finalHandler)
		log.Debug("Registered handler", "pattern", regHandler.Pattern, "match_type", regHandler.MatchType, "middleware_count", len(regHandler.Middleware))
	}

	log.Info("Registered Telegram handlers successfully", "count", len(registeredHandlers))
	return nil
}
