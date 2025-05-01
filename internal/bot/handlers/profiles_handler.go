package handlers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewProfilesHandler creates a handler for the /mrl_profiles command.
func NewProfilesHandler(deps HandlerDeps) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		log := deps.Logger.With("handler", "profiles")

		// Basic validation
		if update.Message == nil || update.Message.From == nil {
			log.DebugContext(ctx, "Ignoring update with nil message or sender")
			return
		}

		chatID := update.Message.Chat.ID
		adminID := update.Message.From.ID

		log.InfoContext(ctx, "Admin requested user profiles list", "admin_user_id", adminID, "chat_id", chatID)

		// Fetch all user profiles
		profilesMap, err := deps.Store.GetAllUserProfiles(ctx)
		if err != nil {
			log.ErrorContext(ctx, "Failed to fetch user profiles", "error", err)
			_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   deps.Config.Messages.ErrorGeneralMsg, // Use general error config
			})
			if sendErr != nil {
				log.ErrorContext(ctx, "Failed to send error message", "error", sendErr)
			}
			return
		}

		// Check if any profiles were found
		if len(profilesMap) == 0 {
			log.InfoContext(ctx, "No user profiles found")
			_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   deps.Config.Messages.ProfilesEmptyMsg, // Use no profiles config
			})
			if sendErr != nil {
				log.ErrorContext(ctx, "Failed to send no profiles message", "error", sendErr)
			}
			return
		}

		// Extract user IDs and sort them
		userIDs := make([]int64, 0, len(profilesMap))
		for id := range profilesMap {
			userIDs = append(userIDs, id)
		}
		sort.Slice(userIDs, func(i, j int) bool {
			return userIDs[i] < userIDs[j]
		})

		// Format the profiles into a message, iterating over sorted IDs
		var sb strings.Builder
		sb.WriteString(deps.Config.Messages.ProfilesHeaderMsg) // Use header config

		for _, userID := range userIDs {
			p := profilesMap[userID]
			// Format each profile line
			aliasesFormatted := strings.ReplaceAll(p.Aliases, ",", ", ")
			traitsFormatted := strings.ReplaceAll(p.Traits, ",", ", ")
			sb.WriteString(fmt.Sprintf("%d | %s | %s | %s | %s | %s\n\n",
				p.UserID,
				aliasesFormatted,
				p.OriginLocation,
				p.CurrentLocation,
				p.AgeRange,
				traitsFormatted,
			))
		}

		// Send the formatted list
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   sb.String(),
		})
		if err != nil {
			log.ErrorContext(ctx, "Failed to send profiles list", "error", err)
			// Optionally send a generic error if sending the list fails
			_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   deps.Config.Messages.ErrorGeneralMsg,
			})
			if sendErr != nil {
				log.ErrorContext(ctx, "Failed to send error message after list failure", "error", sendErr)
			}
		}
	}
}
