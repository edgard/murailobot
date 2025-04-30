package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/edgard/murailobot/internal/database"
)

type mentionHandler struct {
	deps HandlerDeps
}

// NewMentionHandler returns a handler for bot mentions.
func NewMentionHandler(deps HandlerDeps) bot.HandlerFunc {
	return mentionHandler{deps}.Handle
}

// Handle processes incoming mentions using deps from the handler struct.
func (h mentionHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	deps := h.deps
	log := deps.Logger.With("handler", "mention")
	// Bot username and ID are now available via deps.BotUsername and deps.BotID

	// Use the configurable maxHistoryMessages from the database config
	maxHistoryMessages := deps.Config.Database.MaxHistoryMessages
	if maxHistoryMessages <= 0 {
		maxHistoryMessages = 100
		log.WarnContext(context.Background(), "Invalid maxHistoryMessages in config, using default", "default", maxHistoryMessages)
	}

	// --- Use Bot Info from Deps ---
	botUsername := h.deps.Config.Telegram.BotInfo.Username // Use from deps
	botID := h.deps.Config.Telegram.BotInfo.ID             // Use from deps

	// --- Basic Validation ---
	if update.Message == nil || (update.Message.Text == "" && update.Message.Caption == "" && len(update.Message.Photo) == 0) || update.Message.From == nil {
		log.DebugContext(ctx, "Ignoring update with nil message, text, or sender", "update_id", update.ID)
		return
	}

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID
	// Support text, caption, or image-only mentions
	messageText := update.Message.Text
	if messageText == "" {
		messageText = update.Message.Caption
	}
	messageID := update.Message.ID
	mentionString := "@" + botUsername

	// --- Mention Check ---
	isMentioned := false
	if botUsername != "" {
		// Combine text and caption entities for precise mention detection
		allEntities := append([]models.MessageEntity{}, update.Message.Entities...)
		allEntities = append(allEntities, update.Message.CaptionEntities...)
		for _, entity := range allEntities {
			if entity.Type == models.MessageEntityTypeMention {
				if entity.Offset >= 0 && entity.Length > 0 && entity.Offset < len(messageText) {
					endPos := entity.Offset + entity.Length
					if endPos <= len(messageText) && messageText[entity.Offset:endPos] == mentionString {
						isMentioned = true
						break
					}
				}
			}
		}
		// Fallback check
		if !isMentioned && strings.Contains(messageText, mentionString) {
			isMentioned = true
		}
	} else {
		// This case should be rare now, as main.go exits if GetMe fails
		log.WarnContext(ctx, "Bot username is empty in deps, cannot check for mentions", "bot_id", botID)
		return
	}

	if !isMentioned {
		log.DebugContext(ctx, "Bot not mentioned, skipping mention handler logic", "chat_id", chatID)
		return
	}

	log.InfoContext(ctx, "Handling mention", "chat_id", chatID, "user_id", userID, "message_id", messageID)

	// --- Database: Save Incoming Message ---
	incomingMsg := &database.Message{
		ChatID:    chatID,
		UserID:    userID,
		Content:   messageText,
		Timestamp: time.Unix(int64(update.Message.Date), 0),
	}
	if err := deps.Store.SaveMessage(ctx, incomingMsg); err != nil {
		log.ErrorContext(ctx, "Failed to save incoming message", "error", err, "chat_id", chatID, "message_id", messageID)
	} else {
		log.DebugContext(ctx, "Incoming message saved", "db_message_id", incomingMsg.ID, "chat_id", chatID)
	}

	// --- Prepare Prompt ---
	// Keep the full message text intact including mentions
	promptText := strings.TrimSpace(messageText)

	// Only send generic prompt if no text/caption and no photo
	if promptText == "" && len(update.Message.Photo) == 0 {
		log.InfoContext(ctx, "Mention received but prompt is empty", "chat_id", chatID)
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "You mentioned me, but didn't provide a prompt. How can I help?",
		})
		if err != nil {
			log.ErrorContext(ctx, "Failed to send empty prompt message", "error", err, "chat_id", chatID)
		}
		return
	}

	// --- Database: Retrieve History ---
	contextMessages, err := deps.Store.GetRecentMessages(ctx, chatID, maxHistoryMessages, incomingMsg.ID)
	if err != nil {
		log.ErrorContext(ctx, "Failed to retrieve message history", "error", err, "chat_id", chatID)
		contextMessages = []*database.Message{} // Initialize as empty slice instead of nil
	}

	contextMessages = append(contextMessages, incomingMsg)

	// Sort all messages chronologically (only needs to be done once)
	sort.Slice(contextMessages, func(i, j int) bool {
		if contextMessages[i] == nil || contextMessages[j] == nil { // Add nil checks
			return false
		}
		if contextMessages[i].Timestamp.Equal(contextMessages[j].Timestamp) {
			return contextMessages[i].ID < contextMessages[j].ID
		}
		return contextMessages[i].Timestamp.Before(contextMessages[j].Timestamp)
	})

	// Enforce message limit if needed
	if len(contextMessages) > maxHistoryMessages {
		contextMessages = contextMessages[len(contextMessages)-maxHistoryMessages:]
	}

	// Image analysis support on mention
	if len(update.Message.Photo) > 0 {
		processImageMention(ctx, b, deps, chatID, messageID, contextMessages, update.Message.Photo)
		return
	}

	// Fallback to text prompt processing
	processTextMention(ctx, b, deps, chatID, messageID, contextMessages)
}

// downloadPhoto retrieves the file data and detects MIME type.
func downloadPhoto(ctx context.Context, b *bot.Bot, token, fileID string) ([]byte, string, error) {
	// Parameter validation
	if token == "" {
		return nil, "", fmt.Errorf("empty token provided")
	}
	if fileID == "" {
		return nil, "", fmt.Errorf("empty fileID provided")
	}

	// Check for context cancellation before starting
	if ctx.Err() != nil {
		return nil, "", fmt.Errorf("context cancelled before file download: %w", ctx.Err())
	}

	// Create a timeout context for the download operation
	downloadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Fetch file metadata
	fileObj, err := b.GetFile(downloadCtx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get file: %w", err)
	}
	if fileObj.FilePath == "" {
		return nil, "", fmt.Errorf("empty file path returned from Telegram")
	}

	// Download file content using context-aware request
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

	// Close the response body properly and handle any close errors
	defer func() {
		closeErr := resp.Body.Close()
		if closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	// Read the response body with a size limit to prevent potential DoS
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file data: %w", err)
	}

	if len(data) == 0 {
		return nil, "", fmt.Errorf("received empty file data")
	}

	// Detect MIME type from data
	mime := http.DetectContentType(data)
	return data, mime, nil
}

// sendAndSaveReply sends a reply message and saves it to the database.
func sendAndSaveReply(ctx context.Context, b *bot.Bot, deps HandlerDeps, chatID int64, replyTo int, text string) {
	log := deps.Logger.With("handler", "mention")

	// Parameter validation
	if b == nil {
		log.ErrorContext(ctx, "Nil bot instance provided")
		return
	}
	if chatID == 0 {
		log.ErrorContext(ctx, "Invalid chatID provided")
		return
	}
	if replyTo <= 0 {
		log.ErrorContext(ctx, "Invalid messageID to reply to", "reply_to", replyTo)
		return
	}
	if text == "" {
		log.WarnContext(ctx, "Empty text provided for reply")
		text = "I don't have a response at this time."
	}

	// Check for context cancellation
	if ctx.Err() != nil {
		log.ErrorContext(ctx, "Context cancelled before sending reply", "error", ctx.Err())
		return
	}

	// Create a timeout context for sending the message
	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Send the reply message
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

	// Check if we should save the message (valid botID)
	if deps.Config.Telegram.BotInfo.ID == 0 {
		log.WarnContext(ctx, "Invalid botID, skipping message saving")
		return
	}

	// Create a timeout context for database operation
	dbCtx, dbCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dbCancel()

	// Save the sent message to the database
	msg := &database.Message{
		ChatID:    chatID,
		UserID:    deps.Config.Telegram.BotInfo.ID,
		Content:   text,
		Timestamp: time.Now().UTC(),
	}

	if saveErr := deps.Store.SaveMessage(dbCtx, msg); saveErr != nil {
		log.ErrorContext(ctx, "Failed to save bot reply", "error", saveErr)
	} else {
		log.DebugContext(ctx, "Bot reply saved to database", "db_message_id", msg.ID)
	}
}

// deduplicateMessages removes duplicate messages based on ID, keeping the last occurrence.
func deduplicateMessages(messages []*database.Message) []*database.Message {
	if len(messages) <= 1 {
		return messages
	}
	seenIDs := make(map[uint]int) // Map ID to its last index
	for i, msg := range messages {
		if msg != nil { // Add nil check for safety
			seenIDs[msg.ID] = i
		}
	}

	// Create a map to store the unique messages using the last seen index
	uniqueMessagesMap := make(map[int]*database.Message)
	for id, index := range seenIDs {
		if messages[index] != nil && messages[index].ID == id { // Ensure the message at the index matches the ID
			uniqueMessagesMap[index] = messages[index]
		}
	}

	// Extract the unique messages into a slice
	uniqueMessages := make([]*database.Message, 0, len(uniqueMessagesMap))
	indices := make([]int, 0, len(uniqueMessagesMap))
	for index := range uniqueMessagesMap {
		indices = append(indices, index)
	}
	sort.Ints(indices) // Sort by original index to maintain order

	for _, index := range indices {
		uniqueMessages = append(uniqueMessages, uniqueMessagesMap[index])
	}

	return uniqueMessages
}

func processImageMention(ctx context.Context, b *bot.Bot, deps HandlerDeps, chatID int64, messageID int, contextMessages []*database.Message, photoSizes []models.PhotoSize) {
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
	if len(contextMessages) == 0 {
		log.WarnContext(ctx, "Empty context messages for image analysis", "chat_id", chatID)
		// Continue with empty context, as we still have the current message
	}

	// Check for context cancellation
	if ctx.Err() != nil {
		log.ErrorContext(ctx, "Context cancelled before processing image", "error", ctx.Err())
		return
	}

	// Ensure there are photos to process
	if len(photoSizes) == 0 {
		log.ErrorContext(ctx, "No photos to process in message")
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "I couldn't process the image. Please try again."}); sendErr != nil {
			log.ErrorContext(ctx, "Failed to send error message", "error", sendErr)
		}
		return
	}

	// Create timeout context for image processing
	imgCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Download image data for the largest photo
	fileID := photoSizes[len(photoSizes)-1].FileID
	data, mimeType, err := downloadPhoto(imgCtx, b, deps.Config.Telegram.Token, fileID)
	if err != nil {
		log.ErrorContext(ctx, "Photo download failed", "error", err)
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.GeneralError}); sendErr != nil {
			log.ErrorContext(ctx, "Failed to send error message", "error", sendErr)
		}
		return
	}

	// Check for context cancellation before continuing
	if imgCtx.Err() != nil {
		log.ErrorContext(ctx, "Context cancelled after photo download", "error", imgCtx.Err())
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "Sorry, it's taking too long to process your image. Please try again later."}); sendErr != nil {
			log.ErrorContext(ctx, "Failed to send timeout message", "error", sendErr)
		}
		return
	}

	// Indicate typing
	_, _ = b.SendChatAction(ctx, &bot.SendChatActionParams{ChatID: chatID, Action: models.ChatActionTyping})

	// Analyse image with a dedicated timeout context for AI processing
	aiCtx, aiCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer aiCancel()

	finalContextMessages := deduplicateMessages(contextMessages)
	if len(finalContextMessages) != len(contextMessages) {
		log.DebugContext(ctx, "Deduplicated messages before sending to AI", "original_count", len(contextMessages), "final_count", len(finalContextMessages), "chat_id", chatID)
	}

	analysisText, err := deps.GeminiClient.GenerateImageAnalysis(aiCtx, finalContextMessages, mimeType, data, deps.Config.Telegram.BotInfo.ID, deps.Config.Telegram.BotInfo.Username, deps.Config.Telegram.BotInfo.FirstName)
	if err != nil {
		log.ErrorContext(ctx, "Image analysis failed", "error", err)
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.GeneralError}); sendErr != nil {
			log.ErrorContext(ctx, "Failed to send error message", "error", sendErr)
		}
		return
	}

	if analysisText == "" {
		log.WarnContext(ctx, "Empty analysis text received from AI")
		analysisText = "I processed your image but couldn't generate a meaningful response. Could you provide more context?"
	}

	sendAndSaveReply(ctx, b, deps, chatID, messageID, analysisText)
}

// helper for text-based mentions
func processTextMention(ctx context.Context, b *bot.Bot, deps HandlerDeps, chatID int64, messageID int, contextMessages []*database.Message) {
	log := deps.Logger.With("handler", "mention")

	// Parameter validation
	if chatID == 0 {
		log.ErrorContext(ctx, "Invalid chatID provided")
		return
	}
	if messageID <= 0 {
		log.ErrorContext(ctx, "Invalid messageID provided", "message_id", messageID)
		return
	}
	if contextMessages == nil {
		log.WarnContext(ctx, "Nil context messages array provided")
		contextMessages = []*database.Message{} // Convert to empty slice instead of nil
	}

	// Check for context cancellation
	if ctx.Err() != nil {
		log.ErrorContext(ctx, "Context cancelled before processing text mention", "error", ctx.Err())
		return
	}

	// Create a timeout context for AI processing
	aiCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// Indicate typing
	_, _ = b.SendChatAction(ctx, &bot.SendChatActionParams{ChatID: chatID, Action: models.ChatActionTyping})

	finalContextMessages := deduplicateMessages(contextMessages)
	if len(finalContextMessages) != len(contextMessages) {
		log.DebugContext(ctx, "Deduplicated messages before sending to AI", "original_count", len(contextMessages), "final_count", len(finalContextMessages), "chat_id", chatID)
	}

	// Generate reply
	replyText, err := deps.GeminiClient.GenerateReply(aiCtx, finalContextMessages, deps.Config.Telegram.BotInfo.ID, deps.Config.Telegram.BotInfo.Username, deps.Config.Telegram.BotInfo.FirstName)
	if err != nil {
		log.ErrorContext(ctx, "Reply generation failed", "error", err)
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.GeneralError}); sendErr != nil {
			log.ErrorContext(ctx, "Failed to send error message", "error", sendErr)
		}
		return
	}

	// Check if context was cancelled during AI processing
	if aiCtx.Err() != nil {
		log.ErrorContext(ctx, "Context timeout during reply generation", "error", aiCtx.Err())
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "Sorry, it's taking too long to generate a response. Please try again later."}); sendErr != nil {
			log.ErrorContext(ctx, "Failed to send timeout message", "error", sendErr)
		}
		return
	}

	// Validate the AI response
	if replyText == "" {
		log.WarnContext(ctx, "Empty reply text received from AI")
		replyText = "I processed your message but couldn't generate a meaningful response. Could you rephrase your question?"
	}

	// Send and persist reply
	sendAndSaveReply(ctx, b, deps, chatID, messageID, replyText)
}
