package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

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
		log.WarnContext(ctx, "Empty text provided for reply")
		text = "I don't have a response at this time."
	}
	if ctx.Err() != nil {
		log.ErrorContext(ctx, "Context cancelled before sending reply", "error", ctx.Err())
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
		log.ErrorContext(ctx, "Failed to send reply", "error", err)
		return
	}
	log.InfoContext(ctx, "Sent reply", "chat_id", chatID, "message_id", sent.ID)
	if deps.Config.Telegram.BotInfo.ID == 0 {
		log.WarnContext(ctx, "Invalid botID, skipping message saving")
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
		log.ErrorContext(ctx, "AI generation failed", "error", err)
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.GeneralError}); sendErr != nil {
			log.ErrorContext(ctx, "Failed to send error message", "error", sendErr)
		}
		return
	}
	if resp == "" {
		log.WarnContext(ctx, "Empty AI response received")
		resp = "I processed your message but couldn't generate a meaningful response. Could you rephrase or provide more context?"
	}
	SendAndSaveReply(ctx, b, deps, chatID, messageID, resp)
}
