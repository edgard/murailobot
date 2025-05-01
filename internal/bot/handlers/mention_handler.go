package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/edgard/murailobot/internal/database"
)

const (
	photoDownloadTimeout = 30 * time.Second
	aiProcessingTimeout  = 2 * time.Minute
	sendMessageTimeout   = 10 * time.Second
	dbSaveTimeout        = 5 * time.Second
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
		if _, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.MentionNoPromptMsg}); err != nil { // Updated field name
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
		log.ErrorContext(ctx, "No photos to process in message", "chat_id", chatID)
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.MentionImageErrorMsg}); sendErr != nil { // Updated field name
			log.ErrorContext(ctx, "Failed to send error message", "error", sendErr, "chat_id", chatID)
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
		log.ErrorContext(ctx, "Photo download failed", "error", err, "chat_id", chatID)
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.ErrorGeneralMsg}); sendErr != nil { // Updated field name
			log.ErrorContext(ctx, "Failed to send error message", "error", sendErr, "chat_id", chatID)
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

// ---------- Private Helpers ----------

// DownloadPhoto retrieves file data and detects MIME type
func DownloadPhoto(ctx context.Context, b *bot.Bot, token, fileID string) (data []byte, mimeType string, err error) {
	if token == "" {
		return nil, "", fmt.Errorf("empty token provided")
	}
	if fileID == "" {
		return nil, "", fmt.Errorf("empty fileID provided")
	}
	if ctx.Err() != nil {
		return nil, "", fmt.Errorf("context cancelled before file download: %w", ctx.Err())
	}
	downloadCtx, cancel := context.WithTimeout(ctx, photoDownloadTimeout)
	defer cancel()
	fileObj, err := b.GetFile(downloadCtx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get file: %w", err)
	}
	if fileObj.FilePath == "" {
		return nil, "", fmt.Errorf("empty file path returned from Telegram")
	}
	url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", token, fileObj.FilePath)
	req, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download file: %w", err)
	}
	if resp == nil {
		return nil, "", fmt.Errorf("nil response received from HTTP request")
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file data: %w", err)
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("received empty file data")
	}
	mimeType = http.DetectContentType(data)
	return data, mimeType, nil
}

// SendAndSaveReply sends a reply and persists it to the database
func SendAndSaveReply(ctx context.Context, b *bot.Bot, deps HandlerDeps, chatID int64, replyTo int, text string) {
	log := deps.Logger.With("handler", "mention")
	if b == nil || chatID == 0 || replyTo <= 0 {
		log.ErrorContext(ctx, "Invalid parameters to SendAndSaveReply", "chat_id", chatID, "reply_to", replyTo)
		return
	}
	if text == "" {
		log.WarnContext(ctx, "Empty text provided for reply, using fallback", "chat_id", chatID, "reply_to", replyTo)
		text = deps.Config.Messages.MentionEmptyReplyFallbackMsg // Updated field name
	}
	if ctx.Err() != nil {
		log.ErrorContext(ctx, "Context cancelled before sending reply", "error", ctx.Err(), "chat_id", chatID)
		return
	}
	sendCtx, cancel := context.WithTimeout(ctx, sendMessageTimeout)
	defer cancel()
	sent, err := b.SendMessage(sendCtx, &bot.SendMessageParams{
		ChatID:          chatID,
		Text:            text,
		ReplyParameters: &models.ReplyParameters{MessageID: replyTo},
	})
	if err != nil {
		log.ErrorContext(ctx, "Failed to send reply", "error", err, "chat_id", chatID)
		return
	}
	log.InfoContext(ctx, "Sent reply", "chat_id", chatID, "message_id", sent.ID)
	if deps.Config.Telegram.BotInfo.ID == 0 {
		log.WarnContext(ctx, "Invalid botID, skipping message saving", "chat_id", chatID)
		return
	}
	msg := &database.Message{
		ChatID:    chatID,
		UserID:    deps.Config.Telegram.BotInfo.ID,
		Content:   text,
		Timestamp: time.Now().UTC(),
	}
	SaveMessageWithRetry(ctx, deps, msg, "bot reply")
}

// SaveMessageWithRetry attempts to save a message with retries
func SaveMessageWithRetry(ctx context.Context, deps HandlerDeps, msg *database.Message, msgType string) {
	log := deps.Logger.With("handler", "mention")
	const maxRetries = 3
	var err error

	for i := range [maxRetries]struct{}{} {
		// Check if the parent context was cancelled before retrying
		if ctx.Err() != nil {
			log.WarnContext(ctx, fmt.Sprintf("Context cancelled, aborting %s save attempts", msgType),
				"error", ctx.Err(), "chat_id", msg.ChatID, "attempt", i+1)
			return
		}

		dbCtx, cancel := context.WithTimeout(ctx, dbSaveTimeout)
		err = deps.Store.SaveMessage(dbCtx, msg)
		cancel()

		if err == nil {
			log.DebugContext(ctx, fmt.Sprintf("%s saved", msgType), "db_message_id", msg.ID, "chat_id", msg.ChatID)
			return
		}

		log.ErrorContext(ctx, fmt.Sprintf("Failed to save %s, retrying", msgType), "error", err, "chat_id", msg.ChatID, "attempt", i+1)
		time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
	}

	log.ErrorContext(ctx, fmt.Sprintf("Failed to save %s after %d retries", msgType, maxRetries), "error", err, "chat_id", msg.ChatID)
}

// DeduplicateMessages removes duplicate messages, preserving order
func DeduplicateMessages(messages []*database.Message) []*database.Message {
	if len(messages) <= 1 {
		return messages
	}
	unique := make(map[uint]*database.Message)
	idCounter := uint(1)

	for _, m := range messages {
		if m != nil {
			// Handle messages with ID=0 (zero value) by assigning temporary IDs
			// This prevents messages with ID=0 from overwriting each other
			if m.ID == 0 {
				// Assign a temporary unique ID that won't conflict with real IDs
				tmpID := ^uint(0) - idCounter
				idCounter++
				// Store with temporary ID but don't modify the original message
				unique[tmpID] = m
			} else {
				unique[m.ID] = m
			}
		}
	}

	result := make([]*database.Message, 0, len(unique))
	for _, m := range unique {
		result = append(result, m)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Timestamp.Equal(result[j].Timestamp) {
			return result[i].ID < result[j].ID
		}
		return result[i].Timestamp.Before(result[j].Timestamp)
	})

	return result
}

// AIProcess handles common AI request flow
func AIProcess(ctx context.Context, b *bot.Bot, deps HandlerDeps, chatID int64, messageID int, messages []*database.Message, generate func(context.Context, []*database.Message) (string, error)) {
	log := deps.Logger.With("handler", "mention")
	if ctx.Err() != nil || chatID == 0 || messageID <= 0 {
		log.ErrorContext(ctx, "Invalid parameters or cancelled context for AIProcess", "chat_id", chatID, "message_id", messageID)
		return
	}
	_, _ = b.SendChatAction(ctx, &bot.SendChatActionParams{ChatID: chatID, Action: models.ChatActionTyping})
	finalMsgs := DeduplicateMessages(messages)
	if len(finalMsgs) != len(messages) {
		log.DebugContext(ctx, "Deduplicated messages before sending to AI", "original_count", len(messages), "final_count", len(finalMsgs), "chat_id", chatID)
	}
	aiCtx, cancel := context.WithTimeout(ctx, aiProcessingTimeout)
	defer cancel()
	resp, err := generate(aiCtx, finalMsgs)
	if err != nil {
		log.ErrorContext(ctx, "AI generation failed", "error", err, "chat_id", chatID)
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.ErrorGeneralMsg}); sendErr != nil { // Updated field name
			log.ErrorContext(ctx, "Failed to send error message", "error", sendErr, "chat_id", chatID)
		}
		return
	}
	if resp == "" {
		log.WarnContext(ctx, "Empty AI response received, using fallback", "chat_id", chatID, "message_id", messageID)
		resp = deps.Config.Messages.MentionAIEmptyFallbackMsg // Updated field name
	}
	SendAndSaveReply(ctx, b, deps, chatID, messageID, resp)
}
