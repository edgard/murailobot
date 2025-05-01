// Package handlers contains Telegram bot command and message handlers,
// along with their registration logic.
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

// mentionHandler holds dependencies for processing mention events.
type mentionHandler struct {
	deps HandlerDeps
}

// NewMentionHandler returns a handler for bot mentions.
// This handler acts as the default handler when no specific command matches.
func NewMentionHandler(deps HandlerDeps) bot.HandlerFunc {
	return mentionHandler{deps}.Handle
}

// Handle processes incoming messages to check for mentions or replies to the bot.
// It saves the incoming message, retrieves context, and dispatches to either
// text or image processing based on the message content.
func (h mentionHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	deps := h.deps
	log := deps.Logger.With("handler", "mention")

	// Use the configurable maxHistoryMessages from the database config
	maxHistoryMessages := deps.Config.Database.MaxHistoryMessages
	if maxHistoryMessages <= 0 {
		maxHistoryMessages = 100 // Default value if config is invalid
		log.WarnContext(ctx, "Invalid maxHistoryMessages in config, using default", "default", maxHistoryMessages)
	}

	msg := update.Message
	if msg == nil || (msg.Text == "" && msg.Caption == "" && len(msg.Photo) == 0) || msg.From == nil {
		log.DebugContext(ctx, "Ignoring update with nil message, empty content, or nil sender", "update_id", update.ID)
		return
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

	// Consolidate text and caption, preferring text if both exist
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
		// Depending on requirements, might want to return here or proceed cautiously
	}

	// Check if the bot was mentioned, replied to, or otherwise referenced
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

	// Guard against empty prompts after saving the message
	if strings.TrimSpace(text) == "" && len(msg.Photo) == 0 {
		log.InfoContext(ctx, "Mention received but prompt is empty", "chat_id", chatID)
		if _, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.MentionNoPromptMsg}); err != nil {
			log.ErrorContext(ctx, "Failed to send empty prompt message", "error", err, "chat_id", chatID)
		}
		return
	}

	// Retrieve and prepare context messages for the AI
	contextMessages := h.getContextMessages(ctx, chatID, incomingMsg, maxHistoryMessages)

	// Dispatch to image or text processing
	if len(msg.Photo) > 0 {
		h.processImageMention(ctx, b, chatID, msg.ID, contextMessages, msg.Photo)
	} else {
		h.processTextMention(ctx, b, chatID, msg.ID, contextMessages)
	}
}

// shouldHandle returns true if the message warrants a bot response.
// It checks for:
// 1. Entity-based @mentions (ignoring commands).
// 2. Exact word match of the bot's username (case-insensitive, ignoring punctuation).
// 3. Direct replies to a message sent by the bot.
func (h mentionHandler) shouldHandle(msg *models.Message) bool {
	if msg == nil {
		return false
	}
	botID := h.deps.Config.Telegram.BotInfo.ID
	username := h.deps.Config.Telegram.BotInfo.Username
	if username == "" {
		// Cannot reliably check mentions without username
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

	// Combine text and caption, lowercased for case-insensitive matching
	text := strings.ToLower(msgText + " " + msgCaption)
	mention := "@" + strings.ToLower(username)

	// 1. Entity-based @mention check (skipping commands)
	for _, e := range append(msg.Entities, msg.CaptionEntities...) {
		if e.Type == models.MessageEntityTypeBotCommand {
			continue // Ignore commands like /start@BotName
		}
		// Check if the mention entity matches the bot's username
		if e.Type == models.MessageEntityTypeMention && e.Offset >= 0 && e.Length > 0 && e.Offset+e.Length <= len(text) {
			if text[e.Offset:e.Offset+e.Length] == mention {
				return true
			}
		}
	}

	// 2. Exact word match check (strip punctuation)
	for _, w := range strings.Fields(text) {
		stripped := strings.TrimFunc(w, unicode.IsPunct)
		if stripped == strings.ToLower(username) {
			return true
		}
	}

	// 3. Direct reply check
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil && msg.ReplyToMessage.From.ID == botID {
		return true
	}

	return false
}

// getContextMessages retrieves recent message history for the chat, appends the current
// incoming message, sorts them chronologically, and truncates to the specified maxHistory limit.
func (h mentionHandler) getContextMessages(ctx context.Context, chatID int64, incomingMsg *database.Message, maxHistory int) []*database.Message {
	deps := h.deps
	log := deps.Logger.With("handler", "mention")
	msgs, err := deps.Store.GetRecentMessages(ctx, chatID, maxHistory, incomingMsg.ID)
	if err != nil {
		log.ErrorContext(ctx, "Failed to retrieve message history", "error", err, "chat_id", chatID)
		msgs = []*database.Message{} // Use empty slice on error
	}

	// Append the current message to the history
	msgs = append(msgs, incomingMsg)

	// Sort messages by timestamp, then by ID for stability
	sort.Slice(msgs, func(i, j int) bool {
		if msgs[i] == nil || msgs[j] == nil {
			return i < j // Handle potential nils gracefully, though unlikely
		}
		if msgs[i].Timestamp.Equal(msgs[j].Timestamp) {
			return msgs[i].ID < msgs[j].ID // Use DB ID as tie-breaker
		}
		return msgs[i].Timestamp.Before(msgs[j].Timestamp)
	})

	// Truncate if history exceeds the maximum limit
	if len(msgs) > maxHistory {
		return msgs[len(msgs)-maxHistory:]
	}
	return msgs
}

// processTextMention handles mentions that contain only text by delegating to AIProcess.
func (h mentionHandler) processTextMention(ctx context.Context, b *bot.Bot, chatID int64, messageID int, contextMessages []*database.Message) {
	deps := h.deps
	log := deps.Logger.With("handler", "mention")
	log.InfoContext(ctx, "Handling text mention", "chat_id", chatID, "message_id", messageID)
	if contextMessages == nil {
		contextMessages = []*database.Message{} // Ensure non-nil slice
	}

	AIProcess(ctx, b, deps, chatID, messageID, contextMessages,
		func(aiCtx context.Context, msgs []*database.Message) (string, error) {
			// Use GenerateReply for text-based interactions
			return deps.GeminiClient.GenerateReply(aiCtx, msgs,
				deps.Config.Telegram.BotInfo.ID,
				deps.Config.Telegram.BotInfo.Username,
				deps.Config.Telegram.BotInfo.FirstName,
				true, // Include system instructions
			)
		},
	)
}

// processImageMention handles mentions that include an image. It selects the best quality
// photo, downloads it, and then delegates to AIProcess for analysis.
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

	// Find the best quality photo (approximated by largest dimensions)
	var bestPhoto models.PhotoSize
	bestQuality := 0
	for _, photo := range photoSizes {
		quality := photo.Width * photo.Height // Use area as quality heuristic
		if quality > bestQuality {
			bestQuality = quality
			bestPhoto = photo
		}
	}

	fileID := bestPhoto.FileID
	log.DebugContext(ctx, "Selected best quality photo", "file_id", fileID, "width", bestPhoto.Width, "height", bestPhoto.Height)

	// Download the selected image data
	data, mimeType, err := DownloadPhoto(ctx, b, deps.Config.Telegram.Token, fileID)
	if err != nil {
		log.ErrorContext(ctx, "Photo download failed", "error", err, "chat_id", chatID, "file_id", fileID)
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.ErrorGeneralMsg}); sendErr != nil {
			log.ErrorContext(ctx, "Failed to send download error message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	// Delegate to AIProcess for image analysis
	AIProcess(ctx, b, deps, chatID, messageID, contextMessages,
		func(aiCtx context.Context, msgs []*database.Message) (string, error) {
			// Use GenerateImageAnalysis for image-based interactions
			return deps.GeminiClient.GenerateImageAnalysis(aiCtx, msgs, mimeType, data,
				deps.Config.Telegram.BotInfo.ID,
				deps.Config.Telegram.BotInfo.Username,
				deps.Config.Telegram.BotInfo.FirstName,
				true, // Include system instructions
			)
		},
	)
}

// DownloadPhoto retrieves file metadata from Telegram using GetFile, constructs the download URL,
// performs an HTTP GET request with a timeout, reads the response body, and detects the MIME type.
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
	// Ensure body is closed, capturing potential close error
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body from %s: %w", url, closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		// Attempt to read body for more details, but don't overwrite original error if read fails
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) // Limit read size for error body
		return nil, "", fmt.Errorf("unexpected status code %d from %s: %s", resp.StatusCode, url, string(bodyBytes))
	}

	// Limit download size to prevent excessive memory usage
	const maxDownloadSize = 10 * 1024 * 1024 // 10 MB
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

// SendAndSaveReply sends a reply message via the bot API and then attempts to save
// the bot's reply message to the database using SaveMessageWithRetry.
// It uses a fallback message if the provided text is empty.
func SendAndSaveReply(ctx context.Context, b *bot.Bot, deps HandlerDeps, chatID int64, replyTo int, text string) {
	log := deps.Logger.With("handler", "mention")
	if b == nil || chatID == 0 || replyTo <= 0 {
		log.ErrorContext(ctx, "Invalid parameters to SendAndSaveReply", "chat_id", chatID, "reply_to", replyTo)
		return
	}

	// Use fallback message if AI response is empty
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
		return // Don't attempt to save if sending failed
	}

	log.InfoContext(ctx, "Sent reply", "chat_id", chatID, "message_id", sent.ID)

	// Save the bot's reply to the database
	if deps.Config.Telegram.BotInfo.ID == 0 {
		log.WarnContext(ctx, "Invalid botID (0), skipping saving bot reply", "chat_id", chatID)
		return
	}
	msg := &database.Message{
		ChatID:    chatID,
		UserID:    deps.Config.Telegram.BotInfo.ID, // Bot's own user ID
		Content:   text,
		Timestamp: time.Now().UTC(), // Use current time for bot message timestamp
	}
	SaveMessageWithRetry(ctx, deps, msg, "bot reply")
}

// SaveMessageWithRetry attempts to save a message to the database with a fixed number
// of retries and exponential backoff. It respects the parent context for cancellation.
func SaveMessageWithRetry(ctx context.Context, deps HandlerDeps, msg *database.Message, msgType string) {
	log := deps.Logger.With("handler", "mention")
	const maxRetries = 3
	var err error

	for i := range [maxRetries]struct{}{} {
		// Check if the parent context was cancelled before attempting save/retry
		if ctx.Err() != nil {
			log.WarnContext(ctx, fmt.Sprintf("Context cancelled, aborting %s save attempts", msgType),
				"error", ctx.Err(), "chat_id", msg.ChatID, "attempt", i+1)
			return
		}

		dbCtx, cancel := context.WithTimeout(ctx, dbSaveTimeout)
		err = deps.Store.SaveMessage(dbCtx, msg)
		cancel() // Release context resources promptly

		if err == nil {
			log.DebugContext(ctx, fmt.Sprintf("%s saved successfully", msgType), "db_message_id", msg.ID, "chat_id", msg.ChatID)
			return // Success
		}

		// Log error and prepare for retry
		log.ErrorContext(ctx, fmt.Sprintf("Failed to save %s, retrying", msgType), "error", err, "chat_id", msg.ChatID, "attempt", i+1)
		// Exponential backoff: 500ms, 1000ms, 1500ms
		time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
	}

	// Log final failure after all retries
	log.ErrorContext(ctx, fmt.Sprintf("Failed to save %s after %d retries", msgType, maxRetries), "last_error", err, "chat_id", msg.ChatID)
}

// DeduplicateMessages removes duplicate messages based on their database ID, preserving order.
// It handles messages with ID=0 (e.g., unsaved incoming messages) by assigning temporary,
// non-conflicting IDs during processing to ensure they aren't incorrectly deduplicated.
func DeduplicateMessages(messages []*database.Message) []*database.Message {
	if len(messages) <= 1 {
		return messages // No duplicates possible
	}

	unique := make(map[uint]*database.Message)
	// Counter for assigning temporary IDs to messages with ID 0.
	// Start from 1 and use high bits to avoid collision with real DB IDs.
	tempIDCounter := uint(1)

	for _, m := range messages {
		if m != nil {
			if m.ID == 0 {
				// Assign a temporary unique ID starting from the highest possible uint value downwards.
				// This assumes real DB IDs won't reach this range.
				tempID := ^uint(0) - tempIDCounter
				tempIDCounter++
				unique[tempID] = m // Store with temporary ID
			} else {
				unique[m.ID] = m // Use actual DB ID
			}
		}
	}

	// Extract unique messages from the map
	result := make([]*database.Message, 0, len(unique))
	for _, m := range unique {
		result = append(result, m)
	}

	// Sort the unique messages back into chronological order
	sort.Slice(result, func(i, j int) bool {
		if result[i].Timestamp.Equal(result[j].Timestamp) {
			// Use original ID (or 0 for temp ones) as tie-breaker for stability
			return result[i].ID < result[j].ID
		}
		return result[i].Timestamp.Before(result[j].Timestamp)
	})

	return result
}

// AIProcess handles the common workflow for AI interactions: sending typing action,
// deduplicating messages, calling the provided AI generation function with a timeout,
// handling errors or empty responses with fallbacks, and sending/saving the final reply.
func AIProcess(ctx context.Context, b *bot.Bot, deps HandlerDeps, chatID int64, messageID int, messages []*database.Message, generate func(context.Context, []*database.Message) (string, error)) {
	log := deps.Logger.With("handler", "mention")
	if ctx.Err() != nil || chatID == 0 || messageID <= 0 || generate == nil {
		log.ErrorContext(ctx, "Invalid parameters or cancelled context for AIProcess", "chat_id", chatID, "message_id", messageID, "generate_nil", generate == nil)
		return
	}

	// Indicate activity to the user
	_, _ = b.SendChatAction(ctx, &bot.SendChatActionParams{ChatID: chatID, Action: models.ChatActionTyping})

	// Ensure message history is clean before sending to AI
	finalMsgs := DeduplicateMessages(messages)
	if len(finalMsgs) != len(messages) {
		log.DebugContext(ctx, "Deduplicated messages before sending to AI", "original_count", len(messages), "final_count", len(finalMsgs), "chat_id", chatID)
	}

	// Call the specific AI generation function (reply or image analysis) with timeout
	aiCtx, cancel := context.WithTimeout(ctx, aiProcessingTimeout)
	defer cancel()
	resp, err := generate(aiCtx, finalMsgs)
	if err != nil {
		log.ErrorContext(ctx, "AI generation failed", "error", err, "chat_id", chatID)
		// Send a generic error message on AI failure
		if _, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: deps.Config.Messages.ErrorGeneralMsg}); sendErr != nil {
			log.ErrorContext(ctx, "Failed to send AI error message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	// Handle empty AI response with a fallback message
	if resp == "" {
		log.WarnContext(ctx, "Empty AI response received, using fallback", "chat_id", chatID, "message_id", messageID)
		resp = deps.Config.Messages.MentionAIEmptyFallbackMsg
	}

	// Send the final response and save it
	SendAndSaveReply(ctx, b, deps, chatID, messageID, resp)
}
