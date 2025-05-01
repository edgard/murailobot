package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewEditUserHandler returns a handler for the /mrl_edit_user command.
func NewEditUserHandler(deps HandlerDeps) bot.HandlerFunc {
	return editUserHandler{deps}.Handle
}

type editUserHandler struct {
	deps HandlerDeps
}

func (h editUserHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	log := h.deps.Logger.With("handler", "edit_user")
	allowedFields := map[string]bool{
		"aliases":          true,
		"origin_location":  true,
		"current_location": true,
		"age_range":        true,
		"traits":           true,
	}
	// Create a list of allowed field names for error messages
	allowedKeys := make([]string, 0, len(allowedFields))
	for k := range allowedFields {
		allowedKeys = append(allowedKeys, k)
	}
	allowedFieldsStr := strings.Join(allowedKeys, ", ")

	// Ensure Message and From are not nil (should be guaranteed by middleware/handler registration)
	if update.Message == nil || update.Message.From == nil {
		log.ErrorContext(ctx, "EditUser handler called with nil Message or From", "update_id", update.ID)
		return
	}

	chatID := update.Message.Chat.ID
	args := strings.Fields(update.Message.Text) // Split command and arguments

	// 2. Validate Arguments
	if len(args) < 4 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.EditUserUsageMsg,
		})
		if err != nil {
			log.ErrorContext(ctx, "Failed to send usage message", "error", err, "chat_id", chatID)
		}
		return
	}

	userIDStr := args[1]
	fieldName := strings.ToLower(args[2])   // Normalize field name
	newValue := strings.Join(args[3:], " ") // Join the rest as the value

	// Parse UserID
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.EditUserInvalidIDErrorMsg,
		})
		if err != nil {
			log.ErrorContext(ctx, "Failed to send invalid ID message", "error", err, "chat_id", chatID)
		}
		return
	}

	// Validate Field Name
	if !allowedFields[fieldName] {
		// Use config message format string
		replyMsg := fmt.Sprintf(h.deps.Config.Messages.EditUserInvalidFieldFmt, fieldName, allowedFieldsStr)
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   replyMsg,
		})
		if err != nil {
			log.ErrorContext(ctx, "Failed to send invalid field message", "error", err, "chat_id", chatID)
		}
		return
	}

	log.InfoContext(ctx, "Admin requested user profile edit",
		"chat_id", chatID,
		"admin_user_id", update.Message.From.ID,
		"target_user_id", userID,
		"field", fieldName,
		"new_value", newValue,
	)

	// 3. Fetch User Profile
	profile, err := h.deps.Store.GetUserProfile(ctx, userID)
	if err != nil {
		log.ErrorContext(ctx, "Failed to get user profile for editing", "error", err, "target_user_id", userID)
		// Use specific fetch error format
		replyMsg := fmt.Sprintf(h.deps.Config.Messages.EditUserFetchErrorFmt, userID)
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   replyMsg,
		})
		if sendErr != nil {
			log.ErrorContext(ctx, "Failed to send fetch error message", "error", sendErr, "chat_id", chatID)
		}
		return
	}
	if profile == nil {
		// Use config message format string
		replyMsg := fmt.Sprintf(h.deps.Config.Messages.EditUserNotFoundFmt, userID)
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   replyMsg,
		})
		if err != nil {
			log.ErrorContext(ctx, "Failed to send user not found message", "error", err, "chat_id", chatID)
		}
		return
	}

	// 4. Update Profile Field
	originalValue := ""
	switch fieldName {
	case "aliases":
		originalValue = profile.Aliases
		profile.Aliases = newValue
	case "origin_location":
		originalValue = profile.OriginLocation
		profile.OriginLocation = newValue
	case "current_location":
		originalValue = profile.CurrentLocation
		profile.CurrentLocation = newValue
	case "age_range":
		originalValue = profile.AgeRange
		profile.AgeRange = newValue
	case "traits":
		originalValue = profile.Traits
		profile.Traits = newValue
	default:
		// Should not happen due to validation above, but good practice
		log.ErrorContext(ctx, "Internal error: validated field name not handled in switch", "field_name", fieldName)
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.ErrorGeneralMsg, // Use general error config
		})
		if sendErr != nil {
			log.ErrorContext(ctx, "Failed to send error message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	// 5. Save Updated Profile
	err = h.deps.Store.SaveUserProfile(ctx, profile)
	if err != nil {
		log.ErrorContext(ctx, "Failed to save updated user profile", "error", err, "target_user_id", userID)
		// Use specific update error format
		replyMsg := fmt.Sprintf(h.deps.Config.Messages.EditUserUpdateErrorFmt, fieldName)
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   replyMsg,
		})
		if sendErr != nil {
			log.ErrorContext(ctx, "Failed to send update error message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	// 6. Send Confirmation
	log.InfoContext(ctx, "Successfully updated user profile field",
		"target_user_id", userID,
		"field", fieldName,
		"old_value", originalValue,
		"new_value", newValue,
	)
	// Use config message format string
	replyMsg := fmt.Sprintf(h.deps.Config.Messages.EditUserSuccessFmt, fieldName, userID)
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   replyMsg,
	})
	if err != nil {
		log.ErrorContext(ctx, "Failed to send success message", "error", err, "chat_id", chatID)
	}
}

// Deprecated: original newEditUserHandler kept for reference
// func newEditUserHandler(deps HandlerDeps) bot.HandlerFunc { ... }
