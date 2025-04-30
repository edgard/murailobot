// Package handlers provides Telegram command handlers for the bot.
package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewAnalyzeHandler returns a handler for the /mrl_analyze command.
func NewAnalyzeHandler(deps HandlerDeps) bot.HandlerFunc {
	return analyzeHandler{deps}.Handle
}

type analyzeHandler struct {
	deps HandlerDeps
}

func (h analyzeHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	log := h.deps.Logger.With("handler", "analyze")

	// Ensure Message and From are not nil
	if update.Message == nil || update.Message.From == nil {
		log.ErrorContext(ctx, "Analyze handler called with nil Message or From", "update_id", update.ID)
		return
	}

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID
	log.InfoContext(ctx, "Admin requested manual profile analysis", "chat_id", chatID, "user_id", userID)

	// Send initial "Analyzing..." message
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   h.deps.Config.Messages.Analyzing,
	})
	if err != nil {
		log.ErrorContext(ctx, "Failed to send analyzing message", "error", err, "chat_id", chatID)
	}

	// Create a timeout context for the analysis operation
	// This prevents the operation from running indefinitely
	analysisTimeout := 2 * time.Minute // Reasonable timeout for analysis operation
	timeoutCtx, cancel := context.WithTimeout(ctx, analysisTimeout)
	defer cancel() // Ensure resources are cleaned up

	// --- Start of consolidated profile generation logic ---
	var processedCount, savedCount int
	var analysisErr error

	func() { // Use a closure to manage scope and potential panics/errors cleanly
		logicLog := h.deps.Logger.With("logic", "GenerateAndUpdateProfiles") // Use h.deps logger

		// 1. Fetch Unprocessed Messages
		messages, err := h.deps.Store.GetUnprocessedMessages(timeoutCtx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				logicLog.WarnContext(ctx, "Timeout fetching unprocessed messages")
				analysisErr = fmt.Errorf("operation timed out while fetching unprocessed messages")
			} else {
				logicLog.ErrorContext(ctx, "Failed to get unprocessed messages", "error", err)
				analysisErr = fmt.Errorf("failed to get unprocessed messages: %w", err)
			}
			return
		}

		// Check if context is already done (timeout or cancellation)
		if timeoutCtx.Err() != nil {
			logicLog.WarnContext(ctx, "Context cancelled or timed out", "error", timeoutCtx.Err())
			analysisErr = timeoutCtx.Err()
			return
		}

		if len(messages) == 0 {
			logicLog.InfoContext(ctx, "No unprocessed messages found for analysis")
			// processedCount and savedCount remain 0, analysisErr is nil
			return
		}
		logicLog.InfoContext(ctx, "Found messages to analyze", "count", len(messages))

		// 2. Fetch Existing Profiles
		existingProfiles, err := h.deps.Store.GetAllUserProfiles(timeoutCtx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				logicLog.WarnContext(ctx, "Timeout fetching existing user profiles")
				analysisErr = fmt.Errorf("operation timed out while fetching user profiles")
			} else {
				logicLog.ErrorContext(ctx, "Failed to get existing user profiles", "error", err)
				analysisErr = fmt.Errorf("failed to get existing user profiles: %w", err)
			}
			return
		}

		// Check if context is already done
		if timeoutCtx.Err() != nil {
			logicLog.WarnContext(ctx, "Context cancelled or timed out", "error", timeoutCtx.Err())
			analysisErr = timeoutCtx.Err()
			return
		}

		logicLog.InfoContext(ctx, "Fetched existing profiles", "count", len(existingProfiles))

		// 3. Call Gemini to Generate/Update Profiles - This should process all messages in context together
		updatedProfiles, err := h.deps.GeminiClient.GenerateProfiles(timeoutCtx, messages, existingProfiles)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				logicLog.WarnContext(ctx, "Timeout generating profiles via Gemini")
				analysisErr = fmt.Errorf("operation timed out while generating profiles")
			} else {
				logicLog.ErrorContext(ctx, "Failed to generate profiles via Gemini", "error", err)
				analysisErr = fmt.Errorf("failed to generate profiles via Gemini: %w", err)
			}
			// Don't mark messages as processed if Gemini fails
			return
		}

		// Check if context is already done
		if timeoutCtx.Err() != nil {
			logicLog.WarnContext(ctx, "Context cancelled or timed out", "error", timeoutCtx.Err())
			analysisErr = timeoutCtx.Err()
			return
		}

		logicLog.InfoContext(ctx, "Received profiles from Gemini", "count", len(updatedProfiles))

		// 4. Save Updated Profiles - All at once to maintain transaction-like behavior
		var savedProfileCount int
		var erroredProfiles int

		// First check that we have valid profiles to save
		for userID, profile := range updatedProfiles {
			// Check if context is already done - Exit early if timeout occurred
			if timeoutCtx.Err() != nil {
				logicLog.WarnContext(ctx, "Context cancelled or timed out during profile saving",
					"error", timeoutCtx.Err(),
					"processed", savedProfileCount,
					"total", len(updatedProfiles))
				analysisErr = timeoutCtx.Err()
				return
			}

			if profile == nil {
				logicLog.WarnContext(ctx, "Received nil profile from Gemini for user", "user_id", userID)
				erroredProfiles++
				continue
			}

			// Ensure UserID is set correctly
			profile.UserID = userID

			// Save profile
			if err := h.deps.Store.SaveUserProfile(timeoutCtx, profile); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					logicLog.WarnContext(ctx, "Timeout saving profile", "user_id", userID)
					analysisErr = fmt.Errorf("operation timed out while saving profiles")
					return
				}
				logicLog.ErrorContext(ctx, "Failed to save updated profile", "error", err, "user_id", userID)
				erroredProfiles++
			} else {
				savedProfileCount++
			}
		}

		// Only consider the operation successful if we saved at least one profile
		// and didn't have too many errors (using a simple heuristic)
		savedCount = savedProfileCount
		successfulOperation := savedProfileCount > 0 && erroredProfiles < len(updatedProfiles)/2

		// 5. Mark Messages as Processed only if the overall operation was successful
		if successfulOperation {
			// Collect all message IDs to mark as processed
			var messageIDs []uint
			for _, msg := range messages {
				messageIDs = append(messageIDs, msg.ID)
			}

			if len(messageIDs) > 0 {
				// Final check for timeout before marking messages as processed
				if timeoutCtx.Err() != nil {
					logicLog.WarnContext(ctx, "Context cancelled or timed out before marking messages",
						"error", timeoutCtx.Err())
					analysisErr = timeoutCtx.Err()
					return
				}

				err = h.deps.Store.MarkMessagesAsProcessed(timeoutCtx, messageIDs)
				if err != nil {
					if errors.Is(err, context.DeadlineExceeded) {
						logicLog.WarnContext(ctx, "Timeout marking messages as processed")
						analysisErr = fmt.Errorf("operation timed out while marking messages as processed")
					} else {
						logicLog.ErrorContext(ctx, "Failed to mark messages as processed", "error", err)
						analysisErr = fmt.Errorf("failed to mark messages as processed: %w", err)
					}
					return
				}
				logicLog.InfoContext(ctx, "Marked messages as processed", "count", len(messageIDs))
				processedCount = len(messageIDs)
			}
		} else {
			logicLog.WarnContext(ctx, "Profile update operation had too many errors, not marking messages as processed",
				"total_profiles", len(updatedProfiles), "saved_count", savedProfileCount, "errored_count", erroredProfiles)
			processedCount = 0
		}
	}() // End of closure execution

	// --- End of consolidated profile generation logic ---

	// Handle timeout specific error message
	if errors.Is(analysisErr, context.DeadlineExceeded) || errors.Is(analysisErr, context.Canceled) {
		log.WarnContext(ctx, "Analysis operation timed out or was cancelled", "chat_id", chatID)
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "The analysis operation took too long and was automatically cancelled. Try with fewer unprocessed messages.",
		})
		if err != nil {
			log.ErrorContext(ctx, "Failed to send timeout message", "error", err, "chat_id", chatID)
		}
		return
	}

	// Handle other errors
	if analysisErr != nil {
		log.ErrorContext(ctx, "Profile generation logic failed", "error", analysisErr, "chat_id", chatID)
		// Inline sendErrorReply
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.GeneralError,
		})
		if err != nil {
			log.ErrorContext(ctx, "Failed to send error message", "error", err, "chat_id", chatID)
		}
		return
	}

	if processedCount == 0 {
		log.InfoContext(ctx, "No unprocessed messages found or processed by profile generation logic", "chat_id", chatID)
		// Inline sendReply
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.AnalysisNoMessages,
		})
		if err != nil {
			log.ErrorContext(ctx, "Failed to send no messages reply", "error", err, "chat_id", chatID)
		}
		return
	}

	// Send Confirmation
	replyMsg := fmt.Sprintf(h.deps.Config.Messages.AnalysisCompleteFmt, processedCount, savedCount)
	// Inline sendReply
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   replyMsg,
	})
	if err != nil {
		log.ErrorContext(ctx, "Failed to send analysis complete message", "error", err, "chat_id", chatID)
	}
	log.InfoContext(ctx, "Manual profile analysis completed successfully", "processed_count", processedCount, "saved_count", savedCount, "chat_id", chatID)
}

// Deprecated: original newAnalyzeHandler kept for reference, does not affect registry.
// func newAnalyzeHandler(deps HandlerDeps) bot.HandlerFunc { ... }
