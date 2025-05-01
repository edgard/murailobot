// Package handlers contains Telegram bot command and message handlers,
// along with their registration logic.
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

// analyzeHandler holds dependencies for the profile analysis command.
type analyzeHandler struct {
	deps HandlerDeps
}

// Handle processes the /mrl_analyze command, triggering a manual analysis of unprocessed messages.
// It fetches unprocessed messages, gets existing profiles, calls the AI to generate/update profiles,
// saves the results, marks messages as processed (if successful), and reports the outcome.
// This operation has a timeout and includes partial success handling.
// Requires admin privileges (enforced by middleware).
func (h analyzeHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	log := h.deps.Logger.With("handler", "analyze")

	if update.Message == nil || update.Message.From == nil {
		log.ErrorContext(ctx, "Analyze handler called with nil Message or From", "update_id", update.ID)
		return
	}

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID
	log.InfoContext(ctx, "Admin requested profile analysis", "chat_id", chatID, "user_id", userID)

	// Send initial progress message
	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: h.deps.Config.Messages.AnalyzeProgressMsg}); err != nil {
		log.ErrorContext(ctx, "Failed to send analysis progress message", "error", err, "chat_id", chatID)
		// Don't return; attempt to proceed with the analysis anyway.
	}

	// Create a timeout context for the entire analysis operation
	analysisTimeout := 2 * time.Minute // Configurable? Maybe later.
	timeoutCtx, cancel := context.WithTimeout(ctx, analysisTimeout)
	defer cancel() // Ensure resources are cleaned up regardless of outcome

	// --- Start of consolidated profile generation logic ---
	var processedCount, savedCount int
	var analysisErr error

	// Use a closure to manage the complex logic flow, error handling, and context checks cleanly.
	func() {
		logicLog := h.deps.Logger.With("logic", "GenerateAndUpdateProfiles")

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
			return // Exit closure on error
		}

		// Check if context was cancelled or timed out after fetching
		if timeoutCtx.Err() != nil {
			logicLog.WarnContext(ctx, "Context cancelled or timed out after fetching messages", "error", timeoutCtx.Err())
			analysisErr = timeoutCtx.Err()
			return
		}

		if len(messages) == 0 {
			logicLog.InfoContext(ctx, "No unprocessed messages found for analysis")
			// processedCount and savedCount remain 0, analysisErr is nil
			return // Exit closure, no work to do
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
			return // Exit closure on error
		}

		// Check context again after fetching profiles
		if timeoutCtx.Err() != nil {
			logicLog.WarnContext(ctx, "Context cancelled or timed out after fetching profiles", "error", timeoutCtx.Err())
			analysisErr = timeoutCtx.Err()
			return
		}
		logicLog.InfoContext(ctx, "Fetched existing profiles", "count", len(existingProfiles))

		// 3. Call Gemini to Generate/Update Profiles
		updatedProfiles, err := h.deps.GeminiClient.GenerateProfiles(timeoutCtx, messages, existingProfiles)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				logicLog.WarnContext(ctx, "Timeout generating profiles via Gemini")
				analysisErr = fmt.Errorf("operation timed out while generating profiles")
			} else {
				logicLog.ErrorContext(ctx, "Failed to generate profiles via Gemini", "error", err)
				analysisErr = fmt.Errorf("failed to generate profiles via Gemini: %w", err)
			}
			// Do not proceed to save or mark messages processed if Gemini fails.
			return // Exit closure on error
		}

		// Check context after Gemini call
		if timeoutCtx.Err() != nil {
			logicLog.WarnContext(ctx, "Context cancelled or timed out after Gemini generation", "error", timeoutCtx.Err())
			analysisErr = timeoutCtx.Err()
			return
		}
		logicLog.InfoContext(ctx, "Received profiles from Gemini", "count", len(updatedProfiles))

		// 4. Save Updated Profiles
		var savedProfileCount int
		var erroredProfiles int
		for userID, profile := range updatedProfiles {
			// Check context within the loop to allow early exit on timeout during saving
			if timeoutCtx.Err() != nil {
				logicLog.WarnContext(ctx, "Context cancelled or timed out during profile saving loop",
					"error", timeoutCtx.Err(),
					"saved_so_far", savedProfileCount,
					"total_to_save", len(updatedProfiles))
				analysisErr = timeoutCtx.Err()
				return // Exit closure
			}

			if profile == nil {
				logicLog.WarnContext(ctx, "Received nil profile from Gemini for user, skipping save", "user_id", userID)
				erroredProfiles++
				continue
			}

			profile.UserID = userID // Ensure UserID is correctly set

			if err := h.deps.Store.SaveUserProfile(timeoutCtx, profile); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					logicLog.WarnContext(ctx, "Timeout saving profile", "user_id", userID)
					analysisErr = fmt.Errorf("operation timed out while saving profiles")
					return // Exit closure on timeout
				}
				logicLog.ErrorContext(ctx, "Failed to save updated profile", "error", err, "user_id", userID)
				erroredProfiles++
			} else {
				savedProfileCount++
			}
		}
		savedCount = savedProfileCount // Update the outer scope variable

		// Define success criteria (e.g., at least one saved, not too many errors)
		successfulOperation := savedProfileCount > 0 && erroredProfiles < len(updatedProfiles)/2

		// 5. Mark Messages as Processed (only if the overall operation was deemed successful)
		if successfulOperation {
			var messageIDs []uint
			for _, msg := range messages {
				messageIDs = append(messageIDs, msg.ID)
			}

			if len(messageIDs) > 0 {
				// Final context check before the last critical database operation
				if timeoutCtx.Err() != nil {
					logicLog.WarnContext(ctx, "Context cancelled or timed out before marking messages", "error", timeoutCtx.Err())
					analysisErr = timeoutCtx.Err()
					return // Exit closure
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
					return // Exit closure on error
				}
				logicLog.InfoContext(ctx, "Marked messages as processed", "count", len(messageIDs))
				processedCount = len(messageIDs) // Update outer scope variable
			}
		} else {
			logicLog.WarnContext(ctx, "Profile update operation deemed unsuccessful, not marking messages as processed",
				"total_profiles", len(updatedProfiles), "saved_count", savedProfileCount, "errored_count", erroredProfiles)
			processedCount = 0 // Ensure processedCount reflects that messages weren't marked
		}
	}() // End of closure execution

	// --- End of consolidated profile generation logic ---

	// Final reporting based on the outcome of the closure execution

	// Handle timeout errors specifically
	if errors.Is(analysisErr, context.DeadlineExceeded) || errors.Is(analysisErr, context.Canceled) {
		log.WarnContext(ctx, "Profile analysis timed out or was cancelled", "error", analysisErr, "chat_id", chatID)
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.AnalyzeTimeoutMsg,
		})
		if sendErr != nil {
			log.ErrorContext(ctx, "Failed to send timeout message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	// Handle other errors encountered during the process
	if analysisErr != nil {
		log.ErrorContext(ctx, "Profile generation logic failed", "error", analysisErr, "chat_id", chatID)
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.ErrorGeneralMsg,
		})
		if sendErr != nil {
			log.ErrorContext(ctx, "Failed to send general error message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	// Handle case where no messages were found or processed successfully
	if processedCount == 0 && savedCount == 0 { // Check both counts for clarity
		log.InfoContext(ctx, "No unprocessed messages found or processed", "chat_id", chatID)
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.AnalyzeNoMessagesMsg,
		})
		if sendErr != nil {
			log.ErrorContext(ctx, "Failed to send 'no messages' reply", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	// Send completion message with counts
	replyMsg := fmt.Sprintf(h.deps.Config.Messages.AnalyzeCompleteFmt, processedCount, savedCount)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   replyMsg,
	})
	if err != nil {
		log.ErrorContext(ctx, "Failed to send analysis complete message", "error", err, "chat_id", chatID)
	}
	log.InfoContext(ctx, "Manual profile analysis completed", "processed_count", processedCount, "saved_count", savedCount, "chat_id", chatID)
}
