package tasks

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// newProfileAnalysisTask creates a scheduled task that runs profile analysis on unprocessed messages.
// This task automatically processes unprocessed messages and generates/updates user profiles.
func newProfileAnalysisTask(deps TaskDeps) ScheduledTaskFunc {
	log := deps.Logger.With("task", "profile_analysis")

	return func(ctx context.Context) error {
		log.InfoContext(ctx, "Starting scheduled profile analysis task...")
		startTime := time.Now()

		// Set timeout for the entire analysis operation
		analysisTimeout := 5 * time.Minute
		timeoutCtx, cancel := context.WithTimeout(ctx, analysisTimeout)
		defer cancel()

		var processedCount, savedCount int
		var analysisErr error

		// Perform the profile analysis logic
		func() {
			logicLog := deps.Logger.With("logic", "GenerateAndUpdateProfiles")

			messages, err := deps.Store.GetUnprocessedMessages(timeoutCtx)
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

			existingProfiles, err := deps.Store.GetAllUserProfiles(timeoutCtx)
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

			// Get bot info from config
			botID := deps.Config.Telegram.BotInfo.ID
			botUsername := deps.Config.Telegram.BotInfo.Username
			botFirstName := deps.Config.Telegram.BotInfo.FirstName

			updatedProfiles, err := deps.GeminiClient.GenerateProfiles(timeoutCtx, messages, existingProfiles, botID, botUsername, botFirstName)
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

				if err := deps.Store.SaveUserProfile(timeoutCtx, profile); err != nil {
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

					err = deps.Store.MarkMessagesAsProcessed(timeoutCtx, messageIDs)
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

		duration := time.Since(startTime)

		// Handle analysis errors
		if errors.Is(analysisErr, context.DeadlineExceeded) || errors.Is(analysisErr, context.Canceled) {
			log.WarnContext(ctx, "Profile analysis timed out or was cancelled", "error", analysisErr, "duration", duration)
			return fmt.Errorf("profile analysis timed out or was cancelled: %w", analysisErr)
		}

		if analysisErr != nil {
			log.ErrorContext(ctx, "Profile analysis logic failed", "error", analysisErr, "duration", duration)
			return fmt.Errorf("profile analysis failed: %w", analysisErr)
		}

		if processedCount == 0 && savedCount == 0 {
			log.InfoContext(ctx, "Profile analysis completed - no unprocessed messages found", "duration", duration)
		} else {
			log.InfoContext(ctx, "Profile analysis completed successfully",
				"processed_count", processedCount,
				"saved_count", savedCount,
				"duration", duration)
		}

		return nil
	}
}
