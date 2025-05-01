// Package telegram handles the setup of the Telegram bot instance and registration of handlers.
package telegram

import (
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"

	"github.com/edgard/murailobot/internal/bot/handlers"
)

// NewTelegramBot creates a new Telegram bot instance using the go-telegram/bot library.
// It requires a valid bot token and accepts optional bot configuration options.
// If logger is nil, the default slog logger is used.
func NewTelegramBot(token string, logger *slog.Logger, opts ...bot.Option) (*bot.Bot, error) {
	if token == "" {
		return nil, fmt.Errorf("telegram bot token cannot be empty")
	}
	if logger == nil {
		logger = slog.Default() // Use default logger if none provided
	}
	log := logger.With("component", "telegram_bot")

	// Create the bot instance using the provided token and options.
	b, err := bot.New(token, opts...)
	if err != nil {
		log.Error("Failed to create Telegram bot instance", "error", err)
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	// Log success, showing only a prefix of the token for security.
	log.Info("Telegram bot instance created successfully", "token_prefix", token[:min(8, len(token))]+"...")
	return b, nil
}

// applyMiddleware wraps a handler function with a slice of middleware functions.
// Middleware are applied in reverse order of the slice, meaning the first middleware
// in the slice will be the outermost one executed.
func applyMiddleware(handler bot.HandlerFunc, mw []bot.Middleware) bot.HandlerFunc {
	// Iterate backwards through the middleware slice.
	for i := len(mw) - 1; i >= 0; i-- {
		// Wrap the current handler with the middleware.
		handler = mw[i](handler)
	}
	return handler // Return the final handler wrapped with all middleware.
}

// RegisterHandlers registers command and message handlers with the Telegram bot instance.
// It takes a map of RegisteredHandler definitions, applies their associated middleware,
// and registers them with the bot using the specified pattern and match type.
func RegisterHandlers(b *bot.Bot, logger *slog.Logger, registeredHandlers map[string]handlers.RegisteredHandler) error {
	if b == nil {
		return fmt.Errorf("bot instance cannot be nil for handler registration")
	}
	if logger == nil {
		logger = slog.Default()
	}
	log := logger.With("component", "handler_registry")

	if len(registeredHandlers) == 0 {
		log.Warn("No handlers provided for registration.")
		return nil // Not an error, just nothing to do.
	}

	log.Info("Registering Telegram handlers...", "count", len(registeredHandlers))

	// Iterate through the map of handlers to register.
	for name, regHandler := range registeredHandlers {
		if regHandler.Handler == nil {
			log.Warn("Skipping registration for nil handler", "name", name, "pattern", regHandler.Pattern)
			continue // Skip if the handler function itself is nil.
		}

		// Apply middleware associated with this handler.
		finalHandler := applyMiddleware(regHandler.Handler, regHandler.Middleware)

		// Register the handler (with middleware applied) with the bot.
		b.RegisterHandler(regHandler.HandlerType, regHandler.Pattern, regHandler.MatchType, finalHandler)
		log.Debug("Registered handler",
			"name", name,
			"pattern", regHandler.Pattern,
			"type", regHandler.HandlerType,
			"match", regHandler.MatchType,
			"middleware_count", len(regHandler.Middleware))
	}

	log.Info("Registered Telegram handlers successfully", "count", len(registeredHandlers))
	return nil
}
