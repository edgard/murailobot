package handlers

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/edgard/murailobot/internal/database"
)

// ---------- Handler Struct and Constructor ----------
type mentionHandler struct {
	deps HandlerDeps
}

// NewMentionHandler returns a handler for bot mentions.
func NewMentionHandler(deps HandlerDeps) bot.HandlerFunc {
	return mentionHandler{deps}.Handle
}

// ---------- Handle Entry Point ----------
// Handle processes incoming mentions using deps from the handler struct.
func (h mentionHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	deps := h.deps
	log := deps.Logger.With("handler", "mention")

	// Use the configurable maxHistoryMessages from the database config
	maxHistoryMessages := deps.Config.Database.MaxHistoryMessages
	if maxHistoryMessages <= 0 {
		maxHistoryMessages = 100
		log.WarnContext(ctx, "Invalid maxHistoryMessages in config, using default", "default", maxHistoryMessages)
	}

	// Basic Validation and setup
	msg := update.Message
	if msg == nil || (msg.Text == "" && msg.Caption == "" && len(msg.Photo) == 0) || msg.From == nil {
		log.DebugContext(ctx, "Ignoring update with nil message, text, or sender", "update_id", update.ID)
		return
	}

	// Safely handle potentially nil Text and Caption fields, maintaining consistency with shouldHandle
	msgText := ""
	if msg.Text != "" {
		msgText = msg.Text
	}

	msgCaption := ""
	if msg.Caption != "" {
		msgCaption = msg.Caption
	}

	// Consolidate text and caption with a space separator, consistent with shouldHandle
	text := msgText
	if msgText != "" && msgCaption != "" {
		text = msgText + " " + msgCaption
	} else if msgText == "" {
		text = msgCaption
	}

	chatID := msg.Chat.ID
	// Retrieve bot username for mention check
	username := deps.Config.Telegram.BotInfo.Username

	// Mention Check
	if username == "" {
		log.WarnContext(ctx, "Bot username empty, cannot check mentions")
		return
	}

	// Check all forms of interaction on the message
	if !h.shouldHandle(msg) {
		log.DebugContext(ctx, "Bot not mentioned or referenced, skipping mention handler logic", "chat_id", chatID)
		return
	}

	log.DebugContext(ctx, "Handling mention", "chat_id", chatID, "message_id", msg.ID)

	// Save incoming message with retries
	incomingMsg := &database.Message{
		ChatID:    chatID,
		UserID:    msg.From.ID,
		Content:   text,
		Timestamp: time.Unix(int64(msg.Date), 0),
	}

	SaveMessageWithRetry(ctx, deps, incomingMsg, "incoming message")

	// Empty prompt guard
	if strings.TrimSpace(text) == "" && len(msg.Photo) == 0 {
		log.InfoContext(ctx, "Mention received but prompt is empty", "chat_id", chatID)
		if _, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "You mentioned me, but didn't provide a prompt. How can I help?"}); err != nil {
			log.ErrorContext(ctx, "Failed to send empty prompt message", "error", err, "chat_id", chatID)
		}
		return
	}

	// Retrieve and prepare context messages
	contextMessages := h.getContextMessages(ctx, chatID, incomingMsg, maxHistoryMessages)

	// Dispatch to image or text processing
	if len(msg.Photo) > 0 {
		h.processImageMention(ctx, b, chatID, msg.ID, contextMessages, msg.Photo)
	} else {
		h.processTextMention(ctx, b, chatID, msg.ID, contextMessages)
	}
}

// shouldHandle returns true if the message warrants a bot response:
// case-insensitive @ mentions, word matches, replies, or actual forwarding from the bot.
func (h mentionHandler) shouldHandle(msg *models.Message) bool {
	if msg == nil {
		return false
	}
	botID := h.deps.Config.Telegram.BotInfo.ID
	username := h.deps.Config.Telegram.BotInfo.Username
	if username == "" {
		return false
	}

	// Safely handle potentially nil Text and Caption fields
	msgText := ""
	if msg.Text != "" {
		msgText = msg.Text
	}

	msgCaption := ""
	if msg.Caption != "" {
		msgCaption = msg.Caption
	}

	// Combine text and caption, lowercased for case-insensitive matching
	text := strings.ToLower(msgText + " " + msgCaption)
	mention := "@" + strings.ToLower(username)

	// 1. Entity-based @mention, skipping commands
	for _, e := range append(msg.Entities, msg.CaptionEntities...) {
		if e.Type == models.MessageEntityTypeBotCommand {
			continue
		}
		if e.Type == models.MessageEntityTypeMention && e.Offset >= 0 && e.Length > 0 && e.Offset+e.Length <= len(text) {
			if text[e.Offset:e.Offset+e.Length] == mention {
				return true
			}
		}
	}
	// 2. Exact word match (strip punctuation)
	for _, w := range strings.Fields(text) {
		stripped := strings.TrimFunc(w, unicode.IsPunct)
		if stripped == strings.ToLower(username) {
			return true
		}
	}
	// 3. Direct reply to a bot message
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil && msg.ReplyToMessage.From.ID == botID {
		return true
	}
	return false
}

// ---------- Context Helpers ----------
// getContextMessages retrieves, appends, sorts, and truncates message history.
func (h mentionHandler) getContextMessages(ctx context.Context, chatID int64, incomingMsg *database.Message, maxHistory int) []*database.Message {
	deps := h.deps
	log := deps.Logger.With("handler", "mention")
	msgs, err := deps.Store.GetRecentMessages(ctx, chatID, maxHistory, incomingMsg.ID)
	if err != nil {
		log.ErrorContext(ctx, "Failed to retrieve message history", "error", err, "chat_id", chatID)
		msgs = []*database.Message{}
	}
	msgs = append(msgs, incomingMsg)
	sort.Slice(msgs, func(i, j int) bool {
		if msgs[i] == nil || msgs[j] == nil {
			return i < j
		}
		if msgs[i].Timestamp.Equal(msgs[j].Timestamp) {
			return msgs[i].ID < msgs[j].ID
		}
		return msgs[i].Timestamp.Before(msgs[j].Timestamp)
	})
	if len(msgs) > maxHistory {
		return msgs[len(msgs)-maxHistory:]
	}
	return msgs
}

// processTextMention is a method that delegates text-based mentions to aiProcess
func (h mentionHandler) processTextMention(ctx context.Context, b *bot.Bot, chatID int64, messageID int, contextMessages []*database.Message) {
	deps := h.deps
	log := deps.Logger.With("handler", "mention")
	log.InfoContext(ctx, "Handling text mention", "chat_id", chatID, "message_id", messageID)
	if contextMessages == nil {
		contextMessages = []*database.Message{}
	}
	// Delegate to aiProcess with a GenerateReply closure
	AIProcess(ctx, b, deps, chatID, messageID, contextMessages,
		func(aiCtx context.Context, msgs []*database.Message) (string, error) {
			return deps.GeminiClient.GenerateReply(aiCtx, msgs,
				deps.Config.Telegram.BotInfo.ID,
				deps.Config.Telegram.BotInfo.Username,
				deps.Config.Telegram.BotInfo.FirstName,
				true,
			)
		},
	)
}

// processImageMention handles image mentions by downloading the image and delegating AI analysis to aiProcess.
func (h mentionHandler) processImageMention(ctx context.Context, b *bot.Bot, chatID int64, messageID int, contextMessages []*database.Message, photoSizes []models.PhotoSize) {
	deps := h.deps
	log := deps.Logger.With("handler", "mention")
	log.InfoContext(ctx, "Handling image mention", "chat_id", chatID)

	// Parameter validation
	if chatID == 0 {
		log.ErrorContext(ctx, "Invalid chatID provided")
		return
	}
	if messageID <= 0 {
		log.ErrorContext(ctx, "Invalid messageID provided", "message_id", messageID)
		return
	}
	if len(photoSizes) == 0 {
		log.ErrorContext(ctx, "No photos to process in message")
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "I couldn't process the image. Please try again."}); sendErr != nil {
			log.ErrorContext(ctx, "Failed to send error message", "error", sendErr)
		}
		return
	}

	// Find the best quality photo (largest file size)
	var bestPhoto models.PhotoSize
	bestQuality := 0

	for _, photo := range photoSizes {
		// Use width * height as an estimate of quality
		quality := photo.Width * photo.Height
		if quality > bestQuality {
			bestQuality = quality
			bestPhoto = photo
		}
	}

	fileID := bestPhoto.FileID
	log.DebugContext(ctx, "Selected best quality photo", "width", bestPhoto.Width, "height", bestPhoto.Height)

	// Download the selected image
	data, mimeType, err := DownloadPhoto(ctx, b, deps.Config.Telegram.Token, fileID)
	if err != nil {
		log.ErrorContext(ctx, "Photo download failed", "error", err)
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.GeneralError}); sendErr != nil {
			log.ErrorContext(ctx, "Failed to send error message", "error", sendErr)
		}
		return
	}

	// Delegate to aiProcess for image analysis (deduplication is handled there)
	AIProcess(ctx, b, deps, chatID, messageID, contextMessages,
		func(aiCtx context.Context, msgs []*database.Message) (string, error) {
			return deps.GeminiClient.GenerateImageAnalysis(aiCtx, msgs, mimeType, data,
				deps.Config.Telegram.BotInfo.ID,
				deps.Config.Telegram.BotInfo.Username,
				deps.Config.Telegram.BotInfo.FirstName,
				true,
			)
		},
	)
}

// Helper implementations have been moved to mention_helpers.go
