package telegram

import (
	"context"
	stderrors "errors"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/edgard/murailobot/internal/utils"
)

type botCommandHandler struct {
	bot  *bot
	name string
}

func newCommandHandler(bot *bot) ext.Handler {
	return &botCommandHandler{
		bot:  bot,
		name: "botCommandHandler",
	}
}

func (h *botCommandHandler) Name() string {
	return h.name
}

func (h *botCommandHandler) CheckUpdate(_ *gotgbot.Bot, ctx *ext.Context) bool {
	msg := ctx.EffectiveMessage
	return msg != nil && msg.Text != "" && strings.HasPrefix(msg.Text, "/")
}

// validateMessage extracts clean command without bot username suffix
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

// HandleUpdate routes commands:
// /start - Initial greeting
// /mrl - AI chat
// /mrl_reset - Clear history (admin only)
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

func (h *botCommandHandler) handleStart(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if msg == nil {
		return utils.NewError(componentName, utils.ErrOperation, "message is nil", utils.CategoryOperation, nil)
	}

	userID := msg.From.Id
	if !h.isAuthorized(userID) {
		return h.sendUnauthorizedMessage(bot, ctx, userID)
	}

	utils.InfoLog(componentName, "received /start command",
		utils.KeyUserID, userID,
		utils.KeyRequestID, ctx.EffectiveChat.Id,
		utils.KeyName, msg.From.Username,
		utils.KeyAction, "start_command")

	msgCtx, cancel := context.WithTimeout(context.Background(), h.bot.cfg.Telegram.Polling.RequestTimeout)
	defer cancel()

	welcomeMsg := h.bot.cfg.Telegram.Messages.Welcome
	if err := h.sendMessageWithRetry(msgCtx, bot, msg, welcomeMsg); err != nil {
		utils.ErrorLog(componentName, "failed to send welcome message", err,
			utils.KeyUserID, userID,
			utils.KeyRequestID, ctx.EffectiveChat.Id,
			utils.KeyAction, "send_welcome")
		return utils.NewError(componentName, utils.ErrOperation, "failed to send welcome message", utils.CategoryOperation, err)
	}

	return nil
}

// handleChatMessage processes /mrl command:
// 1. Generate AI response
// 2. Save to history
// 3. Send response to user
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

	utils.InfoLog(componentName, "received /mrl command",
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
			utils.ErrorLog(componentName, "failed to send prompt message", err,
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

	response, err := h.bot.ai.Generate(opCtx, userID, username, text)
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

		utils.ErrorLog(componentName, "failed to generate AI response", err,
			utils.KeyUserID, msg.From.Id,
			utils.KeyRequestID, ctx.EffectiveChat.Id,
			utils.KeySize, len(text),
			utils.KeyReason, errorCode,
			utils.KeyAction, "generate_response")

		errMsg = h.bot.ai.Sanitize(errMsg)
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

		if err := h.bot.db.Save(dbCtx, msg.From.Id, username, text, response); err != nil {
			utils.ErrorLog(componentName, "failed to save chat history", err,
				utils.KeyUserID, msg.From.Id,
				utils.KeyRequestID, msg.Chat.Id,
				utils.KeyAction, "save_chat",
				utils.KeyType, "chat_history")
		}

		select {
		case <-opCtx.Done():
			return opCtx.Err()
		default:
			response = h.bot.ai.Sanitize(response)
			msgCtx, cancel := context.WithTimeout(opCtx, h.bot.cfg.Telegram.Polling.RequestTimeout)
			defer cancel()

			return h.sendMessageWithRetry(msgCtx, bot, msg, response)
		}
	}
}

// handleChatHistoryReset is admin-only
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

	utils.InfoLog(componentName, "received /mrl_reset command",
		utils.KeyUserID, userID,
		utils.KeyRequestID, ctx.EffectiveChat.Id,
		utils.KeyName, msg.From.Username,
		utils.KeyType, "admin",
		utils.KeyAction, "reset_command")

	if err := h.bot.db.DeleteAll(opCtx); err != nil {
		utils.ErrorLog(componentName, "failed to reset chat history", err,
			utils.KeyUserID, userID,
			utils.KeyRequestID, ctx.EffectiveChat.Id,
			utils.KeyAction, "reset_history",
			utils.KeyType, "chat_history")

		if err := h.sendMessageWithRetry(opCtx, bot, msg, h.bot.cfg.Telegram.Messages.GeneralError); err != nil {
			return utils.NewError(componentName, utils.ErrOperation, "failed to send error message", utils.CategoryOperation, err)
		}
		return utils.NewError(componentName, utils.ErrOperation, "failed to reset chat history", utils.CategoryOperation, err)
	}

	resetMsg := h.bot.ai.Sanitize(h.bot.cfg.Telegram.Messages.HistoryReset)
	if err := h.sendMessageWithRetry(opCtx, bot, msg, resetMsg); err != nil {
		utils.ErrorLog(componentName, "failed to send reset confirmation message", err,
			utils.KeyUserID, userID,
			utils.KeyRequestID, ctx.EffectiveChat.Id,
			utils.KeyAction, "send_reset_confirm")
		return utils.NewError(componentName, utils.ErrOperation, "failed to send reset confirmation", utils.CategoryOperation, err)
	}

	utils.InfoLog(componentName, "chat history reset completed",
		utils.KeyAction, "reset_history",
		utils.KeyResult, "success",
		utils.KeyUserID, userID,
		utils.KeyRequestID, ctx.EffectiveChat.Id,
		utils.KeyType, "chat_history")
	return nil
}

func (h *botCommandHandler) isAuthorized(userID int64) bool {
	return h.bot.cfg.IsUserAuthorized(userID)
}

func (h *botCommandHandler) sendUnauthorizedMessage(bot *gotgbot.Bot, ctx *ext.Context, userID int64) error {
	utils.WarnLog(componentName, "unauthorized access attempt",
		utils.KeyUserID, userID,
		utils.KeyRequestID, ctx.EffectiveChat.Id,
		utils.KeyReason, "not_authorized",
		utils.KeyAction, "authorization_check")

	msgCtx, cancel := context.WithTimeout(context.Background(), h.bot.cfg.Telegram.Polling.RequestTimeout)
	defer cancel()

	return h.sendMessageWithRetry(msgCtx, bot, ctx.EffectiveMessage, h.bot.cfg.Telegram.Messages.NotAuthorized)
}

// sendMessageWithRetry uses circuit breaker and backoff
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
