// Package logger provides structured logging setup using Go's slog package.
package logger

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewLogger creates a new slog.Logger instance based on configuration.
func NewLogger(levelStr string, jsonOutput bool) *slog.Logger {
	var level slog.Level
	switch levelStr {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo // Default to Info level
	}

	opts := &slog.HandlerOptions{
		Level: level,
		// AddSource: true, // Uncomment to include source file and line number
	}

	var handler slog.Handler
	if jsonOutput {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	return logger
}

// Middleware returns a Telegram bot middleware function for logging updates.
func Middleware(log *slog.Logger) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			startTime := time.Now()

			// Log basic update info before processing
			logEntry := log.With(
				"update_id", update.ID,
				"start_time", startTime.Format(time.RFC3339),
			)

			// Extract more details depending on the update type
			var updateType string
			var chatID int64
			var userID int64
			var text string

			if update.Message != nil {
				updateType = "message"
				chatID = update.Message.Chat.ID
				userID = update.Message.From.ID
				text = update.Message.Text
				logEntry = logEntry.With(
					"message_id", update.Message.ID,
					"chat_id", chatID,
					"user_id", userID,
					"text", text, // Be mindful of logging sensitive info
				)
			} else if update.CallbackQuery != nil {
				updateType = "callback_query"
				userID = update.CallbackQuery.From.ID
				text = update.CallbackQuery.Data
				logEntry = logEntry.With(
					"callback_query_id", update.CallbackQuery.ID,
					"user_id", userID,
					"data", text,
				)

				// Access embedded fields explicitly to resolve ambiguity.
				// Check the Date field via the embedded Message struct.
				if update.CallbackQuery.Message.Message.Date != 0 {
					// Accessible Message (Date != 0)
					// Access Chat field via the embedded Message struct. Chat is a struct, not a pointer.
					chatID = update.CallbackQuery.Message.Message.Chat.ID
					logEntry = logEntry.With("chat_id", chatID, "accessible", true)
				} else {
					// Inaccessible Message (Date == 0)
					// Access Chat field via the embedded InaccessibleMessage struct. Chat is a struct.
					chatID = update.CallbackQuery.Message.InaccessibleMessage.Chat.ID
					logEntry = logEntry.With("chat_id", chatID, "accessible", false)
				}
			} else {
				updateType = "other"
				// Add logging for other update types if needed (edited_message, channel_post, etc.)
			}
			logEntry = logEntry.With("update_type", updateType)

			logEntry.InfoContext(ctx, "Processing update")

			// Call the next handler in the chain
			next(ctx, b, update)

			// Log after processing
			duration := time.Since(startTime)
			logEntry.InfoContext(ctx, "Finished processing update", "duration", duration)
		}
	}
}
