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
// It supports different log levels (debug, info, warn, error) and JSON/text output formats.
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
		level = slog.LevelInfo // Default to Info level if config is invalid
	}

	opts := &slog.HandlerOptions{
		Level: level,
		// AddSource: true, // Uncomment to include source file and line number in logs (useful for debugging)
	}

	var handler slog.Handler
	if jsonOutput {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger) // Set this logger as the default for the application
	return logger
}

// Middleware returns a Telegram bot middleware function that logs incoming updates
// and the time taken to process them.
func Middleware(log *slog.Logger) bot.Middleware {
	// This function returns the actual middleware handler.
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		// This is the handler function executed for each update.
		// It logs details about the update before and after calling the next handler.
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			startTime := time.Now()

			// Prepare base log entry with common fields.
			logEntry := log.With(
				"update_id", update.ID,
				"start_time", startTime.Format(time.RFC3339),
			)

			// Extract and log specific details based on the update type.
			var updateType string
			var chatID int64
			var userID int64
			var text string // Holds message text or callback data

			if update.Message != nil {
				updateType = "message"
				chatID = update.Message.Chat.ID
				if update.Message.From != nil { // User might be nil for channel posts forwarded to groups
					userID = update.Message.From.ID
				}
				text = update.Message.Text
				logEntry = logEntry.With(
					"message_id", update.Message.ID,
					"chat_id", chatID,
					"user_id", userID,
					"text_preview", truncateString(text, 50), // Log a preview, avoid logging potentially large/sensitive text
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

				// CallbackQuery.Message can be either *models.Message or *models.InaccessibleMessage.
				// We need to check which one it is to correctly access Chat.ID.
				// The `Date` field is only present in the accessible `Message` struct.
				if update.CallbackQuery.Message.Message.Date != 0 {
					// Message is accessible (type *models.Message)
					chatID = update.CallbackQuery.Message.Message.Chat.ID
					logEntry = logEntry.With("chat_id", chatID, "message_accessible", true)
				} else {
					// Message is inaccessible (type *models.InaccessibleMessage)
					chatID = update.CallbackQuery.Message.InaccessibleMessage.Chat.ID
					logEntry = logEntry.With("chat_id", chatID, "message_accessible", false)
				}
			} else {
				updateType = "other"
				// Consider adding logging for other relevant update types if needed
				// (e.g., edited_message, channel_post, inline_query).
			}
			logEntry = logEntry.With("update_type", updateType)

			logEntry.InfoContext(ctx, "Processing update")

			// Execute the actual handler logic.
			next(ctx, b, update)

			// Log completion details.
			duration := time.Since(startTime)
			logEntry.InfoContext(ctx, "Finished processing update", "duration", duration)
		}
	}
}

// truncateString limits a string to a maximum length, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return "..." // Not enough space for meaningful truncation
	}
	return s[:maxLen-3] + "..."
}
