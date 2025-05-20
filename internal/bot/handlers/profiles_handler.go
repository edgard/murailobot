package handlers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewProfilesHandler creates a handler for the /mrl_profiles command that displays
// information about all stored user profiles to administrators.
func NewProfilesHandler(deps HandlerDeps) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		log := deps.Logger.With("handler", "profiles")

		if update.Message == nil || update.Message.From == nil {
			log.DebugContext(ctx, "Ignoring update with nil message or sender")
			return
		}

		chatID := update.Message.Chat.ID
		adminID := update.Message.From.ID

		log.InfoContext(ctx, "Admin requested user profiles list", "admin_user_id", adminID, "chat_id", chatID)

		profilesMap, err := deps.Store.GetAllUserProfiles(ctx)
		if err != nil {
			log.ErrorContext(ctx, "Failed to fetch user profiles", "error", err)
			_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   deps.Config.Messages.ErrorGeneralMsg,
			})
			if sendErr != nil {
				log.ErrorContext(ctx, "Failed to send error message after profile fetch failure", "error", sendErr)
			}
			return
		}

		if len(profilesMap) == 0 {
			log.InfoContext(ctx, "No user profiles found in the database")
			_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   deps.Config.Messages.ProfilesEmptyMsg,
			})
			if sendErr != nil {
				log.ErrorContext(ctx, "Failed to send 'no profiles' message", "error", sendErr)
			}
			return
		}

		userIDs := make([]int64, 0, len(profilesMap))
		for id := range profilesMap {
			userIDs = append(userIDs, id)
		}
		sort.Slice(userIDs, func(i, j int) bool {
			return userIDs[i] < userIDs[j]
		})

		var sb strings.Builder
		sb.WriteString(deps.Config.Messages.ProfilesHeaderMsg)

		for _, userID := range userIDs {
			if userID == b.ID() {
				continue
			}
			p := profilesMap[userID]

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

		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   sb.String(),
		})
		if err != nil {
			log.ErrorContext(ctx, "Failed to send profiles list message", "error", err)

			_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   deps.Config.Messages.ErrorGeneralMsg,
			})
			if sendErr != nil {
				log.ErrorContext(ctx, "Failed to send error message after list send failure", "error", sendErr)
			}
		}
	}
}
