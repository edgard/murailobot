// Package logger provides structured logging functionality for MurailoBot.
// It uses Go's slog package for logging with configurable levels and formats.
package logger

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewLogger creates a new slog Logger with the specified level and format.
// If jsonOutput is true, logs will be formatted as JSON, otherwise as text.
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
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if jsonOutput {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// Middleware creates a logging middleware for the Telegram bot.
// It logs information about incoming updates and messages for debugging purposes.
func Middleware(log *slog.Logger) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			startTime := time.Now()

			logEntry := log.With(
				"update_id", update.ID,
				"start_time", startTime.Format(time.RFC3339),
			)

			var updateType string
			var chatID int64
			var userID int64
			var text string

			if update.Message != nil {
				updateType = "message"
				chatID = update.Message.Chat.ID
				if update.Message.From != nil {
					userID = update.Message.From.ID
				}
				text = update.Message.Text
				logEntry = logEntry.With(
					"message_id", update.Message.ID,
					"chat_id", chatID,
					"user_id", userID,
					"text_preview", truncateString(text, 50),
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

				if update.CallbackQuery.Message.Message.Date != 0 {
					chatID = update.CallbackQuery.Message.Message.Chat.ID
					logEntry = logEntry.With("chat_id", chatID, "message_accessible", true)
				} else {
					chatID = update.CallbackQuery.Message.InaccessibleMessage.Chat.ID
					logEntry = logEntry.With("chat_id", chatID, "message_accessible", false)
				}
			} else {
				updateType = "other"
			}
			logEntry = logEntry.With("update_type", updateType)

			logEntry.InfoContext(ctx, "Processing update")

			next(ctx, b, update)

			duration := time.Since(startTime)
			logEntry.InfoContext(ctx, "Finished processing update", "duration", duration)
		}
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return "..."
	}
	return s[:maxLen-3] + "..."
}
