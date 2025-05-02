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

type mentionHandler struct {
	deps HandlerDeps
}

// NewMentionHandler creates a handler that responds to messages where the bot is mentioned.
// It processes the message content and generates responses using the AI client.
func NewMentionHandler(deps HandlerDeps) bot.HandlerFunc {
	return mentionHandler{deps}.Handle
}

func (h mentionHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	deps := h.deps
	log := deps.Logger.With("handler", "mention")

	maxHistoryMessages := deps.Config.Database.MaxHistoryMessages
	if maxHistoryMessages <= 0 {
		maxHistoryMessages = 100
		log.WarnContext(ctx, "Invalid maxHistoryMessages in config, using default", "default", maxHistoryMessages)
	}

	msg := update.Message
	if msg == nil || (msg.Text == "" && msg.Caption == "" && len(msg.Photo) == 0) || msg.From == nil {
		log.DebugContext(ctx, "Ignoring update with nil message, empty content, or nil sender", "update_id", update.ID)
		return
	}

	msgText := ""
	if msg.Text != "" {
		msgText = msg.Text
	}
	msgCaption := ""
	if msg.Caption != "" {
		msgCaption = msg.Caption
	}

	text := msgText
	if msgText != "" && msgCaption != "" {
		text = msgText + " " + msgCaption
	} else if msgText == "" {
		text = msgCaption
	}

	chatID := msg.Chat.ID
	username := deps.Config.Telegram.BotInfo.Username
	if username == "" {
		log.WarnContext(ctx, "Bot username empty in config, cannot check mentions reliably")
	}

	if !h.shouldHandle(msg) {
		log.DebugContext(ctx, "Bot not mentioned or referenced, skipping mention handler logic", "chat_id", chatID)
		return
	}

	log.DebugContext(ctx, "Handling mention", "chat_id", chatID, "message_id", msg.ID)

	incomingMsg := &database.Message{
		ChatID:    chatID,
		UserID:    msg.From.ID,
		Content:   text,
		Timestamp: time.Unix(int64(msg.Date), 0),
	}
	SaveMessageWithRetry(ctx, deps, incomingMsg, "incoming message")

	if strings.TrimSpace(text) == "" && len(msg.Photo) == 0 {
		log.InfoContext(ctx, "Mention received but prompt is empty", "chat_id", chatID)
		if _, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.MentionNoPromptMsg}); err != nil {
			log.ErrorContext(ctx, "Failed to send empty prompt message", "error", err, "chat_id", chatID)
		}
		return
	}

	contextMessages := h.getContextMessages(ctx, chatID, incomingMsg, maxHistoryMessages)

	if len(msg.Photo) > 0 {
		h.processImageMention(ctx, b, chatID, msg.ID, contextMessages, msg.Photo)
	} else {
		h.processTextMention(ctx, b, chatID, msg.ID, contextMessages)
	}
}

func (h mentionHandler) shouldHandle(msg *models.Message) bool {
	if msg == nil {
		return false
	}
	botID := h.deps.Config.Telegram.BotInfo.ID
	username := h.deps.Config.Telegram.BotInfo.Username
	if username == "" {
		return false
	}

	msgText := ""
	if msg.Text != "" {
		msgText = msg.Text
	}
	msgCaption := ""
	if msg.Caption != "" {
		msgCaption = msg.Caption
	}

	text := strings.ToLower(msgText + " " + msgCaption)
	mention := "@" + strings.ToLower(username)

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

	for _, w := range strings.Fields(text) {
		stripped := strings.TrimFunc(w, unicode.IsPunct)
		if stripped == strings.ToLower(username) {
			return true
		}
	}

	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil && msg.ReplyToMessage.From.ID == botID {
		return true
	}

	return false
}

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

func (h mentionHandler) processTextMention(ctx context.Context, b *bot.Bot, chatID int64, messageID int, contextMessages []*database.Message) {
	deps := h.deps
	log := deps.Logger.With("handler", "mention")
	log.InfoContext(ctx, "Handling text mention", "chat_id", chatID, "message_id", messageID)
	if contextMessages == nil {
		contextMessages = []*database.Message{}
	}

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

func (h mentionHandler) processImageMention(ctx context.Context, b *bot.Bot, chatID int64, messageID int, contextMessages []*database.Message, photoSizes []models.PhotoSize) {
	deps := h.deps
	log := deps.Logger.With("handler", "mention")
	log.InfoContext(ctx, "Handling image mention", "chat_id", chatID)

	if chatID == 0 || messageID <= 0 {
		log.ErrorContext(ctx, "Invalid parameters for image processing", "chat_id", chatID, "message_id", messageID)
		return
	}
	if len(photoSizes) == 0 {
		log.ErrorContext(ctx, "No photos provided in message for processing", "chat_id", chatID)
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.MentionImageErrorMsg}); sendErr != nil {
			log.ErrorContext(ctx, "Failed to send image error message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	var bestPhoto models.PhotoSize
	bestQuality := 0
	for _, photo := range photoSizes {
		quality := photo.Width * photo.Height
		if quality > bestQuality {
			bestQuality = quality
			bestPhoto = photo
		}
	}

	fileID := bestPhoto.FileID
	log.DebugContext(ctx, "Selected best quality photo", "file_id", fileID, "width", bestPhoto.Width, "height", bestPhoto.Height)

	data, mimeType, err := DownloadPhoto(ctx, b, deps.Config.Telegram.Token, fileID)
	if err != nil {
		log.ErrorContext(ctx, "Photo download failed", "error", err, "chat_id", chatID, "file_id", fileID)
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.ErrorGeneralMsg}); sendErr != nil {
			log.ErrorContext(ctx, "Failed to send download error message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

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

// DownloadPhoto downloads a photo from Telegram's API using the provided file ID.
// It returns the photo data, detected MIME type, and any error encountered.
func DownloadPhoto(ctx context.Context, b *bot.Bot, token, fileID string) (data []byte, mimeType string, err error) {
	if token == "" {
		return nil, "", fmt.Errorf("empty token provided for photo download")
	}
	if fileID == "" {
		return nil, "", fmt.Errorf("empty fileID provided for photo download")
	}
	if ctx.Err() != nil {
		return nil, "", fmt.Errorf("context cancelled before file download: %w", ctx.Err())
	}

	downloadCtx, cancel := context.WithTimeout(ctx, photoDownloadTimeout)
	defer cancel()

	fileObj, err := b.GetFile(downloadCtx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get file info from Telegram: %w", err)
	}
	if fileObj.FilePath == "" {
		return nil, "", fmt.Errorf("empty file path returned from Telegram for file ID %s", fileID)
	}

	url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", token, fileObj.FilePath)
	req, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create HTTP request for %s: %w", url, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download file from %s: %w", url, err)
	}
	if resp == nil {
		return nil, "", fmt.Errorf("nil HTTP response received from %s", url)
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body from %s: %w", url, closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, "", fmt.Errorf("unexpected status code %d from %s: %s", resp.StatusCode, url, string(bodyBytes))
	}

	const maxDownloadSize = 10 * 1024 * 1024
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxDownloadSize))
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file data from %s: %w", url, err)
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("received empty file data from %s", url)
	}

	mimeType = http.DetectContentType(data)
	return data, mimeType, nil
}

// SendAndSaveReply sends a reply message to the chat and saves it in the database.
// It manages both the Telegram API call and the persistent storage of the bot's response.
func SendAndSaveReply(ctx context.Context, b *bot.Bot, deps HandlerDeps, chatID int64, replyTo int, text string) {
	log := deps.Logger.With("handler", "mention")
	if b == nil || chatID == 0 || replyTo <= 0 {
		log.ErrorContext(ctx, "Invalid parameters to SendAndSaveReply", "chat_id", chatID, "reply_to", replyTo)
		return
	}

	if text == "" {
		log.WarnContext(ctx, "Empty text provided for reply, using fallback", "chat_id", chatID, "reply_to", replyTo)
		text = deps.Config.Messages.MentionEmptyReplyFallbackMsg
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
		log.ErrorContext(ctx, "Failed to send reply message", "error", err, "chat_id", chatID)
		return
	}

	log.InfoContext(ctx, "Sent reply", "chat_id", chatID, "message_id", sent.ID)

	if deps.Config.Telegram.BotInfo.ID == 0 {
		log.WarnContext(ctx, "Invalid botID (0), skipping saving bot reply", "chat_id", chatID)
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

// SaveMessageWithRetry attempts to save a message to the database with retry logic.
// It handles failures and logs appropriate warning messages.
func SaveMessageWithRetry(ctx context.Context, deps HandlerDeps, msg *database.Message, msgType string) {
	log := deps.Logger.With("handler", "mention")
	const maxRetries = 3
	var err error

	for i := range [maxRetries]struct{}{} {
		if ctx.Err() != nil {
			log.WarnContext(ctx, fmt.Sprintf("Context cancelled, aborting %s save attempts", msgType),
				"error", ctx.Err(), "chat_id", msg.ChatID, "attempt", i+1)
			return
		}

		dbCtx, cancel := context.WithTimeout(ctx, dbSaveTimeout)
		err = deps.Store.SaveMessage(dbCtx, msg)
		cancel()

		if err == nil {
			log.DebugContext(ctx, fmt.Sprintf("%s saved successfully", msgType), "db_message_id", msg.ID, "chat_id", msg.ChatID)
			return
		}

		log.ErrorContext(ctx, fmt.Sprintf("Failed to save %s, retrying", msgType), "error", err, "chat_id", msg.ChatID, "attempt", i+1)

		time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
	}

	log.ErrorContext(ctx, fmt.Sprintf("Failed to save %s after %d retries", msgType, maxRetries), "last_error", err, "chat_id", msg.ChatID)
}

// DeduplicateMessages removes duplicate messages from a message slice based on content and timestamp.
// It's used to clean up message history before processing by the AI.
func DeduplicateMessages(messages []*database.Message) []*database.Message {
	if len(messages) <= 1 {
		return messages
	}

	unique := make(map[uint]*database.Message)

	tempIDCounter := uint(1)

	for _, m := range messages {
		if m != nil {
			if m.ID == 0 {
				tempID := ^uint(0) - tempIDCounter
				tempIDCounter++
				unique[tempID] = m
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

// AIProcess handles the AI processing workflow, invoking the provided generate function
// and managing response generation, error handling, and message sending.
func AIProcess(ctx context.Context, b *bot.Bot, deps HandlerDeps, chatID int64, messageID int, messages []*database.Message, generate func(context.Context, []*database.Message) (string, error)) {
	log := deps.Logger.With("handler", "mention")
	if ctx.Err() != nil || chatID == 0 || messageID <= 0 || generate == nil {
		log.ErrorContext(ctx, "Invalid parameters or cancelled context for AIProcess", "chat_id", chatID, "message_id", messageID, "generate_nil", generate == nil)
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

		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.ErrorGeneralMsg}); sendErr != nil {
			log.ErrorContext(ctx, "Failed to send AI error message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	if resp == "" {
		log.WarnContext(ctx, "Empty AI response received, using fallback", "chat_id", chatID, "message_id", messageID)
		resp = deps.Config.Messages.MentionAIEmptyFallbackMsg
	}

	SendAndSaveReply(ctx, b, deps, chatID, messageID, resp)
}
