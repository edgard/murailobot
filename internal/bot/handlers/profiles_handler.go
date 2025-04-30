package handlers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewProfilesHandler returns a handler for the /mrl_profiles command.
func NewProfilesHandler(deps HandlerDeps) bot.HandlerFunc {
	return profilesHandler{deps}.Handle
}

// profilesHandler processes the /mrl_profiles command.
type profilesHandler struct {
	deps HandlerDeps
}

func (h profilesHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	log := h.deps.Logger.With("handler", "profiles")

	// Ensure Message and From are not nil (should be guaranteed by middleware/handler registration)
	if update.Message == nil || update.Message.From == nil {
		log.ErrorContext(ctx, "Profiles handler called with nil Message or From", "update_id", update.ID)
		return
	}

	chatID := update.Message.Chat.ID
	log.InfoContext(ctx, "Admin requested user profiles list", "chat_id", chatID, "user_id", update.Message.From.ID)

	// 2. Fetch All Profiles
	profilesMap, err := h.deps.Store.GetAllUserProfiles(ctx)
	if err != nil {
		log.ErrorContext(ctx, "Failed to get all user profiles", "error", err, "chat_id", chatID)
		// Inline sendErrorReply
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.GeneralError,
		})
		if sendErr != nil {
			log.ErrorContext(ctx, "Failed to send error message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	// 3. Check if profiles exist
	if len(profilesMap) == 0 {
		log.InfoContext(ctx, "No user profiles found in database", "chat_id", chatID)
		// Inline sendReply
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.NoProfiles,
		})
		if sendErr != nil {
			log.ErrorContext(ctx, "Failed to send no profiles message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	// 4. Format Profiles
	var profileStrings []string
	// Sort by UserID for consistent output
	userIDs := make([]int64, 0, len(profilesMap))
	for uid := range profilesMap {
		userIDs = append(userIDs, uid)
	}
	sort.Slice(userIDs, func(i, j int) bool { return userIDs[i] < userIDs[j] })

	for _, uid := range userIDs {
		profile := profilesMap[uid]
		if profile != nil {
			if profile.UserID == h.deps.Config.Telegram.BotInfo.ID {
				continue
			}

			// Helper function to replace empty strings with "Unknown"
			unknownIfEmpty := func(s string) string {
				if strings.TrimSpace(s) == "" {
					return "Unknown"
				}
				return s
			}
			profileStrings = append(profileStrings, fmt.Sprintf("UID %d | %s | %s | %s | %s | %s",
				profile.UserID, unknownIfEmpty(profile.Aliases),
				unknownIfEmpty(profile.OriginLocation),
				unknownIfEmpty(profile.CurrentLocation),
				unknownIfEmpty(profile.AgeRange),
				unknownIfEmpty(profile.Traits)))
		}
	}

	// 5. Send Formatted Reply
	var replyBuilder strings.Builder
	replyBuilder.WriteString(h.deps.Config.Messages.ProfilesHeader)
	replyBuilder.WriteString(strings.Join(profileStrings, "\n"))

	// Send the reply directly without splitting
	fullReply := replyBuilder.String()
	log.DebugContext(ctx, "Sending profiles list", "length", len(fullReply), "chat_id", chatID)

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   fullReply,
	})
	if err != nil {
		log.ErrorContext(ctx, "Failed to send profiles list", "error", err, "chat_id", chatID)
		// Inline sendErrorReply
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.GeneralError,
		})
		if sendErr != nil {
			log.ErrorContext(ctx, "Failed to send error message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	log.InfoContext(ctx, "Successfully sent user profiles list", "count", len(profilesMap), "chat_id", chatID)
}

// Deprecated: original newProfilesHandler kept for reference.
// func newProfilesHandler(deps HandlerDeps) bot.HandlerFunc { ... }
