// Package telegram provides Telegram bot command handling functionality.
package telegram

import (
	"context"
	stderrors "errors"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/edgard/murailobot/internal/utils"
)

// botCommandHandler implements the command handling logic for the Telegram bot.
// It processes incoming commands, manages user authorization, and coordinates
// between the AI service and database operations.
type botCommandHandler struct {
	bot  *bot   // Reference to the main bot instance
	name string // Handler name for debugging
}

// newCommandHandler creates a new command handler instance.
// It implements the ext.Handler interface for processing Telegram updates.
func newCommandHandler(bot *bot) ext.Handler {
	return &botCommandHandler{
		bot:  bot,
		name: "botCommandHandler",
	}
}

// Name returns the handler's identifier for debugging and logging purposes.
func (h *botCommandHandler) Name() string {
	return h.name
}

// CheckUpdate implements ext.Handler interface.
// It determines if an update contains a valid command that this handler
// should process (messages starting with '/').
func (h *botCommandHandler) CheckUpdate(_ *gotgbot.Bot, ctx *ext.Context) bool {
	msg := ctx.EffectiveMessage
	return msg != nil && msg.Text != "" && strings.HasPrefix(msg.Text, "/")
}

// validateMessage checks if a message contains a valid command.
// It performs the following validations:
// - Message exists and is not empty
// - Message starts with '/'
// - Command is properly formatted
// Returns the clean command without the bot username suffix.
func (h *botCommandHandler) validateMessage(msg *gotgbot.Message) (string, error) {
	if msg == nil {
		return "", utils.NewError(componentName, utils.ErrOperation, "message is nil", utils.CategoryOperation, nil)
	}
	if msg.Text == "" {
		return "", nil
	}

	text := strings.TrimSpace(msg.Text)
	if !strings.HasPrefix(text, "/") {
		return "", nil
	}

	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", nil
	}

	cmd := strings.Split(strings.TrimPrefix(parts[0], "/"), "@")[0]
	return cmd, nil
}

// HandleUpdate implements ext.Handler interface.
// It processes incoming commands and routes them to the appropriate handler:
// - /start: Initial bot interaction
// - /mrl: Generate AI response
// - /mrl_reset: Clear chat history (admin only)
// For AI-related commands, it maintains a typing indicator while processing.
func (h *botCommandHandler) HandleUpdate(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveMessage != nil && ctx.EffectiveMessage.Text != "" && strings.HasPrefix(ctx.EffectiveMessage.Text, "/") {
		opCtx, cancel := context.WithTimeout(context.Background(), h.bot.cfg.Telegram.AIRequestTimeout)
		typingCtx, cancelTyping := context.WithCancel(opCtx)
		go h.bot.SendContinuousTyping(typingCtx, b, ctx.EffectiveChat.Id)

		defer func() {
			cancelTyping()
			cancel()
		}()
	}

	cmd, err := h.validateMessage(ctx.EffectiveMessage)
	if err != nil {
		return err
	}
	if cmd == "" {
		return nil
	}

	switch cmd {
	case "start":
		return h.handleStart(b, ctx)
	case "mrl":
		return h.handleChatMessage(b, ctx)
	case "mrl_reset":
		return h.handleChatHistoryReset(b, ctx)
	}
	return nil
}

// handleStart processes the /start command.
// It sends a welcome message if the user is authorized.
// This is typically the first interaction a user has with the bot.
func (h *botCommandHandler) handleStart(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if msg == nil {
		return utils.NewError(componentName, utils.ErrOperation, "message is nil", utils.CategoryOperation, nil)
	}

	userID := msg.From.Id
	if !h.isAuthorized(userID) {
		return h.sendUnauthorizedMessage(bot, ctx, userID)
	}

	utils.WriteInfoLog(componentName, "received /start command",
		utils.KeyUserID, userID,
		utils.KeyRequestID, ctx.EffectiveChat.Id,
		utils.KeyName, msg.From.Username,
		utils.KeyAction, "start_command")

	msgCtx, cancel := context.WithTimeout(context.Background(), h.bot.cfg.Telegram.Polling.RequestTimeout)
	defer cancel()

	welcomeMsg := h.bot.cfg.Telegram.Messages.Welcome
	if err := h.sendMessageWithRetry(msgCtx, bot, msg, welcomeMsg); err != nil {
		utils.WriteErrorLog(componentName, "failed to send welcome message", err,
			utils.KeyUserID, userID,
			utils.KeyRequestID, ctx.EffectiveChat.Id,
			utils.KeyAction, "send_welcome")
		return utils.NewError(componentName, utils.ErrOperation, "failed to send welcome message", utils.CategoryOperation, err)
	}

	return nil
}

// handleChatMessage processes the /mrl command.
// This is the main command for interacting with the AI:
// 1. Validates user authorization
// 2. Processes the message text
// 3. Generates AI response
// 4. Saves the interaction to chat history
// 5. Sends the response back to the user
// The entire operation is protected by timeouts and circuit breakers.
func (h *botCommandHandler) handleChatMessage(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if msg == nil {
		return utils.NewError(componentName, utils.ErrOperation, "message is nil", utils.CategoryOperation, nil)
	}

	userID := msg.From.Id
	if !h.isAuthorized(userID) {
		return h.sendUnauthorizedMessage(bot, ctx, userID)
	}

	opCtx, cancel := context.WithTimeout(context.Background(), h.bot.cfg.Telegram.AIRequestTimeout)
	defer cancel()

	utils.WriteInfoLog(componentName, "received /mrl command",
		utils.KeyUserID, userID,
		utils.KeyRequestID, ctx.EffectiveChat.Id,
		utils.KeyName, msg.From.Username,
		utils.KeyType, "message",
		utils.KeyAction, "chat_command",
		utils.KeySize, len(msg.Text))

	text := strings.TrimSpace(msg.Text)
	cmdParts := strings.Fields(text)
	if len(cmdParts) == 0 {
		return nil
	}

	cmd := strings.Split(cmdParts[0], "@")[0]
	if cmd == "/mrl" && len(cmdParts) == 1 {
		msgCtx, cancel := context.WithTimeout(opCtx, h.bot.cfg.Telegram.Polling.RequestTimeout)
		defer cancel()

		if err := h.sendMessageWithRetry(msgCtx, bot, msg, h.bot.cfg.Telegram.Messages.ProvideMessage); err != nil {
			utils.WriteErrorLog(componentName, "failed to send prompt message", err,
				utils.KeyUserID, msg.From.Id,
				utils.KeyRequestID, ctx.EffectiveChat.Id,
				utils.KeyAction, "send_prompt")
			return utils.NewError(componentName, utils.ErrOperation, "failed to send prompt message", utils.CategoryOperation, err)
		}
		return nil
	}

	text = strings.TrimSpace(strings.TrimPrefix(text, cmdParts[0]))

	username := strings.TrimSpace(msg.From.Username)
	if username == "" {
		username = "Unknown"
	}

	response, err := h.bot.ai.GenerateResponse(opCtx, userID, username, text)
	if err != nil {
		var errMsg string
		var appErr *utils.AppError
		if stderrors.As(err, &appErr) {
			switch appErr.Code {
			case utils.ErrAPI:
				errMsg = h.bot.cfg.Telegram.Messages.AIError
			case utils.ErrValidation:
				errMsg = "Invalid request. Please try again with different input."
			default:
				errMsg = h.bot.cfg.Telegram.Messages.GeneralError
			}
		} else if err == context.DeadlineExceeded {
			errMsg = "Request timed out. Please try again."
		} else {
			errMsg = h.bot.cfg.Telegram.Messages.GeneralError
		}

		errorCode := "unknown"
		if appErr != nil {
			errorCode = appErr.Code
		} else if err == context.DeadlineExceeded {
			errorCode = utils.ErrTimeout
		}

		utils.WriteErrorLog(componentName, "failed to generate AI response", err,
			utils.KeyUserID, msg.From.Id,
			utils.KeyRequestID, ctx.EffectiveChat.Id,
			utils.KeySize, len(text),
			utils.KeyReason, errorCode,
			utils.KeyAction, "generate_response")

		errMsg = h.bot.ai.SanitizeResponse(errMsg)
		msgCtx, cancel := context.WithTimeout(opCtx, h.bot.cfg.Telegram.Polling.RequestTimeout)
		defer cancel()

		if err := h.sendMessageWithRetry(msgCtx, bot, msg, errMsg); err != nil {
			return utils.NewError(componentName, utils.ErrOperation, "failed to send error message", utils.CategoryOperation, err)
		}
		return err
	}

	select {
	case <-opCtx.Done():
		return opCtx.Err()
	default:
		dbCtx, cancel := context.WithTimeout(opCtx, h.bot.cfg.Telegram.DBOperationTimeout)
		defer cancel()

		if err := h.bot.db.SaveChatInteraction(dbCtx, msg.From.Id, username, text, response); err != nil {
			utils.WriteErrorLog(componentName, "failed to save chat history", err,
				utils.KeyUserID, msg.From.Id,
				utils.KeyRequestID, msg.Chat.Id,
				utils.KeyAction, "save_chat",
				utils.KeyType, "chat_history")
		}

		select {
		case <-opCtx.Done():
			return opCtx.Err()
		default:
			response = h.bot.ai.SanitizeResponse(response)
			msgCtx, cancel := context.WithTimeout(opCtx, h.bot.cfg.Telegram.Polling.RequestTimeout)
			defer cancel()

			return h.sendMessageWithRetry(msgCtx, bot, msg, response)
		}
	}
}

// handleChatHistoryReset processes the /mrl_reset command.
// This admin-only command clears all chat history from the database.
// It includes proper authorization checks and confirmation messages.
func (h *botCommandHandler) handleChatHistoryReset(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if msg == nil {
		return utils.NewError(componentName, utils.ErrOperation, "message is nil", utils.CategoryOperation, nil)
	}

	userID := msg.From.Id
	if userID != h.bot.cfg.Telegram.AdminID {
		return h.sendUnauthorizedMessage(bot, ctx, userID)
	}

	opCtx, cancel := context.WithTimeout(context.Background(), h.bot.cfg.Telegram.DBOperationTimeout)
	defer cancel()

	utils.WriteInfoLog(componentName, "received /mrl_reset command",
		utils.KeyUserID, userID,
		utils.KeyRequestID, ctx.EffectiveChat.Id,
		utils.KeyName, msg.From.Username,
		utils.KeyType, "admin",
		utils.KeyAction, "reset_command")

	if err := h.bot.db.DeleteAllChatHistory(opCtx); err != nil {
		utils.WriteErrorLog(componentName, "failed to reset chat history", err,
			utils.KeyUserID, userID,
			utils.KeyRequestID, ctx.EffectiveChat.Id,
			utils.KeyAction, "reset_history",
			utils.KeyType, "chat_history")

		if err := h.sendMessageWithRetry(opCtx, bot, msg, h.bot.cfg.Telegram.Messages.GeneralError); err != nil {
			return utils.NewError(componentName, utils.ErrOperation, "failed to send error message", utils.CategoryOperation, err)
		}
		return utils.NewError(componentName, utils.ErrOperation, "failed to reset chat history", utils.CategoryOperation, err)
	}

	resetMsg := h.bot.ai.SanitizeResponse(h.bot.cfg.Telegram.Messages.HistoryReset)
	if err := h.sendMessageWithRetry(opCtx, bot, msg, resetMsg); err != nil {
		utils.WriteErrorLog(componentName, "failed to send reset confirmation message", err,
			utils.KeyUserID, userID,
			utils.KeyRequestID, ctx.EffectiveChat.Id,
			utils.KeyAction, "send_reset_confirm")
		return utils.NewError(componentName, utils.ErrOperation, "failed to send reset confirmation", utils.CategoryOperation, err)
	}

	utils.WriteInfoLog(componentName, "chat history reset completed",
		utils.KeyAction, "reset_history",
		utils.KeyResult, "success",
		utils.KeyUserID, userID,
		utils.KeyRequestID, ctx.EffectiveChat.Id,
		utils.KeyType, "chat_history")
	return nil
}

// Helper methods

// isAuthorized checks if a user is allowed to interact with the bot
// based on the configured access control lists.
func (h *botCommandHandler) isAuthorized(userID int64) bool {
	return h.bot.cfg.IsUserAuthorized(userID)
}

// sendUnauthorizedMessage sends an access denied message to unauthorized users.
// It includes logging for security monitoring.
func (h *botCommandHandler) sendUnauthorizedMessage(bot *gotgbot.Bot, ctx *ext.Context, userID int64) error {
	utils.WriteWarnLog(componentName, "unauthorized access attempt",
		utils.KeyUserID, userID,
		utils.KeyRequestID, ctx.EffectiveChat.Id,
		utils.KeyReason, "not_authorized",
		utils.KeyAction, "authorization_check")

	msgCtx, cancel := context.WithTimeout(context.Background(), h.bot.cfg.Telegram.Polling.RequestTimeout)
	defer cancel()

	return h.sendMessageWithRetry(msgCtx, bot, ctx.EffectiveMessage, h.bot.cfg.Telegram.Messages.NotAuthorized)
}

// sendMessageWithRetry sends a message to Telegram with retry and circuit breaking.
// It handles transient failures and backs off appropriately to avoid rate limiting.
func (h *botCommandHandler) sendMessageWithRetry(ctx context.Context, bot *gotgbot.Bot, msg *gotgbot.Message, text string) error {
	return h.bot.breaker.Execute(ctx, func(ctx context.Context) error {
		return h.bot.withRetry(ctx, func(ctx context.Context) error {
			_, err := msg.Reply(bot, text, nil)
			if err != nil {
				return utils.NewError(componentName, utils.ErrOperation, "failed to send message", utils.CategoryOperation, err)
			}
			return nil
		})
	})
}
