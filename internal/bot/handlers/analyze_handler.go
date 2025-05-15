// Package handlers implements command and message handlers for the Telegram bot.
// It includes handlers for commands, mentions, and administrative operations.
package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewAnalyzeHandler creates a handler for the /mrl_analyze command that processes unanalyzed messages
// and generates/updates user profiles based on message content.
func NewAnalyzeHandler(deps HandlerDeps) bot.HandlerFunc {
	return analyzeHandler{deps}.handle
}

type analyzeHandler struct {
	deps HandlerDeps
}

func (h analyzeHandler) handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	log := h.deps.Logger.With("handler", "analyze")

	if update.Message == nil || update.Message.From == nil {
		log.ErrorContext(ctx, "Analyze handler called with nil Message or From", "update_id", update.ID)
		return
	}

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID
	log.InfoContext(ctx, "Admin requested profile analysis", "chat_id", chatID, "user_id", userID)

	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: h.deps.Config.Messages.AnalyzeProgressMsg}); err != nil {
		log.ErrorContext(ctx, "Failed to send analysis progress message", "error", err, "chat_id", chatID)
	}

	analysisTimeout := 2 * time.Minute
	timeoutCtx, cancel := context.WithTimeout(ctx, analysisTimeout)
	defer cancel()

	var processedCount, savedCount int
	var analysisErr error

	func() {
		logicLog := h.deps.Logger.With("logic", "GenerateAndUpdateProfiles")

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

		if timeoutCtx.Err() != nil {
			logicLog.WarnContext(ctx, "Context cancelled or timed out after fetching messages", "error", timeoutCtx.Err())
			analysisErr = timeoutCtx.Err()
			return
		}

		if len(messages) == 0 {
			logicLog.InfoContext(ctx, "No unprocessed messages found for analysis")

			return
		}
		logicLog.InfoContext(ctx, "Found messages to analyze", "count", len(messages))

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

		if timeoutCtx.Err() != nil {
			logicLog.WarnContext(ctx, "Context cancelled or timed out after fetching profiles", "error", timeoutCtx.Err())
			analysisErr = timeoutCtx.Err()
			return
		}
		logicLog.InfoContext(ctx, "Fetched existing profiles", "count", len(existingProfiles))

		updatedProfiles, err := h.deps.GeminiClient.GenerateProfiles(timeoutCtx, messages, existingProfiles)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				logicLog.WarnContext(ctx, "Timeout generating profiles via Gemini")
				analysisErr = fmt.Errorf("operation timed out while generating profiles")
			} else {
				logicLog.ErrorContext(ctx, "Failed to generate profiles via Gemini", "error", err)
				analysisErr = fmt.Errorf("failed to generate profiles via Gemini: %w", err)
			}

			return
		}

		if timeoutCtx.Err() != nil {
			logicLog.WarnContext(ctx, "Context cancelled or timed out after Gemini generation", "error", timeoutCtx.Err())
			analysisErr = timeoutCtx.Err()
			return
		}
		logicLog.InfoContext(ctx, "Received profiles from Gemini", "count", len(updatedProfiles))

		var savedProfileCount int
		var erroredProfiles int
		for userID, profile := range updatedProfiles {
			if timeoutCtx.Err() != nil {
				logicLog.WarnContext(ctx, "Context cancelled or timed out during profile saving loop",
					"error", timeoutCtx.Err(),
					"saved_so_far", savedProfileCount,
					"total_to_save", len(updatedProfiles))
				analysisErr = timeoutCtx.Err()
				return
			}

			if profile == nil {
				logicLog.WarnContext(ctx, "Received nil profile from Gemini for user, skipping save", "user_id", userID)
				erroredProfiles++
				continue
			}

			profile.UserID = userID

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
		savedCount = savedProfileCount

		successfulOperation := savedProfileCount > 0 && erroredProfiles < len(updatedProfiles)/2

		if successfulOperation {
			var messageIDs []uint
			for _, msg := range messages {
				messageIDs = append(messageIDs, msg.ID)
			}

			if len(messageIDs) > 0 {
				if timeoutCtx.Err() != nil {
					logicLog.WarnContext(ctx, "Context cancelled or timed out before marking messages", "error", timeoutCtx.Err())
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
			logicLog.WarnContext(ctx, "Profile update operation deemed unsuccessful, not marking messages as processed",
				"total_profiles", len(updatedProfiles), "saved_count", savedProfileCount, "errored_count", erroredProfiles)
			processedCount = 0
		}
	}()

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

	if processedCount == 0 && savedCount == 0 {
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
