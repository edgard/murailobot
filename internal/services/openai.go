package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/common"
	"github.com/edgard/murailobot/internal/interfaces"
	"github.com/edgard/murailobot/internal/models"
	"github.com/sashabaranov/go-openai"
)

// OpenAIConfig holds configuration for OpenAI service
type OpenAIConfig struct {
	Token              string
	BaseURL            string
	Model              string
	MaxTokens          int
	Temperature        float32
	Timeout            time.Duration
	Instruction        string
	ProfileInstruction string
	Bot                interfaces.Bot
}

// OpenAI implements the AI interface using OpenAI's API
type OpenAI struct {
	client    *openai.Client
	config    OpenAIConfig
	maxTokens int
}

// NewOpenAI creates a new OpenAI service instance
func NewOpenAI(config OpenAIConfig) (interfaces.AI, error) {
	if config.Token == "" {
		return nil, common.ErrMissingToken
	}

	if config.Model == "" {
		config.Model = openai.GPT3Dot5Turbo
	}

	if config.Temperature == 0 {
		config.Temperature = 0.7
	}

	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	if config.MaxTokens == 0 {
		config.MaxTokens = 4000
	}

	if config.Bot == nil {
		return nil, common.ErrBotRequired
	}

	aiConfig := openai.DefaultConfig(config.Token)
	if config.BaseURL != "" {
		aiConfig.BaseURL = config.BaseURL
	}

	slog.Info("initializing OpenAI service",
		"model", config.Model)

	return &OpenAI{
		client:    openai.NewClientWithConfig(aiConfig),
		config:    config,
		maxTokens: config.MaxTokens,
	}, nil
}

// createSystemPrompt generates the system prompt for the AI model
func (o *OpenAI) createSystemPrompt(userProfiles map[int64]*models.UserProfile) string {
	botInfo := o.config.Bot.GetInfo()
	displayName := botInfo.Username
	if botInfo.DisplayName != "" {
		displayName = botInfo.DisplayName
	}

	botIdentityHeader := fmt.Sprintf(
		"You are %s, a Telegram bot in a group chat. When someone mentions you with @%s, "+
			"your task is to respond to their message. Messages will include the @%s mention - "+
			"this is normal and expected. Always respond directly to the content of the message. "+
			"Even if the message doesn't contain a clear question, assume it's directed at you "+
			"and respond appropriately.\n\n",
		displayName, botInfo.Username, botInfo.Username)

	systemPrompt := botIdentityHeader + o.config.Instruction

	if len(userProfiles) > 0 {
		var profileInfo strings.Builder
		profileInfo.WriteString("\n\n## USER PROFILES\n")
		profileInfo.WriteString("Format: UID [user_id] ([display_names]) | [origin_location] | [current_location] | [age_range] | [traits]\n\n")

		userIDs := make([]int64, 0, len(userProfiles))
		for id := range userProfiles {
			userIDs = append(userIDs, id)
		}
		sort.Slice(userIDs, func(i, j int) bool {
			return userIDs[i] < userIDs[j]
		})

		for _, id := range userIDs {
			profile := userProfiles[id]
			if id == botInfo.UserID {
				botDisplayNames := botInfo.Username
				if botInfo.DisplayName != "" && botInfo.DisplayName != botInfo.Username {
					botDisplayNames = fmt.Sprintf("%s, %s", botInfo.DisplayName, botInfo.Username)
				}
				profileInfo.WriteString(fmt.Sprintf("UID %d (%s) | Internet | Internet | N/A | Group Chat Bot\n",
					id, botDisplayNames))
				continue
			}
			profileInfo.WriteString(fmt.Sprintf("UID %d (%s) | %s | %s | %s | %s\n",
				profile.UserID, profile.DisplayNames, profile.OriginLocation,
				profile.CurrentLocation, profile.AgeRange, profile.Traits))
		}

		systemPrompt += profileInfo.String()
	}

	return systemPrompt
}

// GenerateResponse generates an AI response for messages
func (o *OpenAI) GenerateResponse(ctx context.Context, messages []*models.Message) (string, error) {
	if len(messages) == 0 {
		return "", common.ErrEmptyInput
	}

	// Get system prompt and count its tokens
	userProfiles := make(map[int64]*models.UserProfile)
	systemPrompt := o.createSystemPrompt(userProfiles)
	systemTokens, err := common.CountTokens(systemPrompt)
	if err != nil {
		return "", fmt.Errorf("%w: %v", common.ErrTokenCount, err)
	}

	// Calculate available tokens for messages
	availableTokens := o.maxTokens - systemTokens

	// Format messages for filtering
	formattedMsgs := make([]string, len(messages))
	for i, msg := range messages {
		formattedMsgs[i] = fmt.Sprintf("[%s] UID %d: %s",
			msg.CreatedAt.Format(time.RFC3339),
			msg.UserID,
			msg.Content)
	}

	// Filter messages to fit within available tokens
	filteredMsgs, err := common.FilterContent(formattedMsgs, availableTokens, true) // true for newest first
	if err != nil {
		return "", fmt.Errorf("%w: %v", common.ErrTokenCount, err)
	}

	// Create chat messages
	chatMessages := []openai.ChatCompletionMessage{{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	}}

	// Add filtered messages
	for i, content := range filteredMsgs {
		role := openai.ChatMessageRoleUser
		if messages[len(messages)-len(filteredMsgs)+i].IsFromBot {
			role = openai.ChatMessageRoleAssistant
		}

		chatMessages = append(chatMessages, openai.ChatCompletionMessage{
			Role:    role,
			Content: content,
		})
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, o.config.Timeout)
	defer cancel()

	resp, err := o.client.CreateChatCompletion(
		timeoutCtx,
		openai.ChatCompletionRequest{
			Model:       o.config.Model,
			Messages:    chatMessages,
			Temperature: o.config.Temperature,
		},
	)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return "", fmt.Errorf("%w: %v", common.ErrAPITimeout, err)
		}
		return "", fmt.Errorf("%w: %v", common.ErrNoResponse, err)
	}

	if len(resp.Choices) == 0 {
		return "", common.ErrNoResponse
	}

	return resp.Choices[0].Message.Content, nil
}

// GenerateProfile generates a user profile from messages
func (o *OpenAI) GenerateProfile(ctx context.Context, userID int64, messages []*models.Message) (*models.UserProfile, error) {
	if len(messages) == 0 {
		return nil, common.ErrEmptyInput
	}

	if userID <= 0 {
		return nil, common.ErrInvalidUserID
	}

	botInfo := o.config.Bot.GetInfo()

	// Create instruction
	instruction := fmt.Sprintf(`
## BOT IDENTIFICATION [IMPORTANT]
Bot UID: %d
Bot Username: %s
Bot Display Name: %s

### BOT INFLUENCE AWARENESS [IMPORTANT]
- DO NOT attribute traits based on topics introduced by the bot
- If the bot mentions a topic and the user merely responds, this is not evidence of a personal trait
- Only identify traits from topics and interests the user has independently demonstrated
- Ignore creative embellishments that might have been added by the bot in previous responses

## OUTPUT FORMAT [CRITICAL]
Return ONLY a JSON object with this structure:
{
    "display_names": "Comma-separated list of names/nicknames",
    "origin_location": "Where the user is from",
    "current_location": "Where the user currently lives",
    "age_range": "Approximate age range (20s, 30s, etc.)",
    "traits": "Comma-separated list of personality traits and characteristics"
}

%s`, botInfo.UserID, botInfo.Username, botInfo.DisplayName, o.config.ProfileInstruction)

	// Calculate available tokens after instruction
	systemTokens, err := common.CountTokens(instruction)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", common.ErrTokenCount, err)
	}
	availableTokens := o.maxTokens - systemTokens

	// Format user's messages
	formattedMsgs := make([]string, 0, len(messages))
	for _, msg := range messages {
		if msg.UserID == userID {
			content := fmt.Sprintf("[%s] %s\n",
				msg.CreatedAt.Format(time.RFC3339),
				msg.Content)
			formattedMsgs = append(formattedMsgs, content)
		}
	}

	// Filter messages to fit within available tokens
	filteredMsgs, err := common.FilterContent(formattedMsgs, availableTokens, true) // true for newest first
	if err != nil {
		return nil, fmt.Errorf("%w: %v", common.ErrTokenCount, err)
	}

	// Create message content
	var messageContent strings.Builder
	messageContent.WriteString(fmt.Sprintf("Analyzing messages from User %d:\n\n", userID))
	for _, content := range filteredMsgs {
		messageContent.WriteString(content)
	}

	chatMessages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: instruction,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: messageContent.String(),
		},
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, o.config.Timeout)
	defer cancel()

	resp, err := o.client.CreateChatCompletion(
		timeoutCtx,
		openai.ChatCompletionRequest{
			Model:       o.config.Model,
			Messages:    chatMessages,
			Temperature: o.config.Temperature,
		},
	)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, fmt.Errorf("%w: %v", common.ErrAPITimeout, err)
		}
		return nil, fmt.Errorf("%w: %v", common.ErrNoResponse, err)
	}

	if len(resp.Choices) == 0 {
		return nil, common.ErrNoResponse
	}

	response := strings.TrimSpace(resp.Choices[0].Message.Content)

	// Extract JSON content
	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return nil, common.ErrInvalidJSON
	}

	jsonContent := response[jsonStart : jsonEnd+1]
	var profile models.UserProfile
	if err := json.Unmarshal([]byte(jsonContent), &profile); err != nil {
		return nil, fmt.Errorf("%w: %v", common.ErrInvalidJSON, err)
	}

	profile.UserID = userID
	profile.LastUpdated = time.Now().UTC()

	// Special handling for bot's own profile
	if userID == botInfo.UserID {
		botDisplayNames := botInfo.Username
		if botInfo.DisplayName != "" && botInfo.DisplayName != botInfo.Username {
			botDisplayNames = fmt.Sprintf("%s, %s", botInfo.DisplayName, botInfo.Username)
		}
		profile = models.UserProfile{
			UserID:          userID,
			DisplayNames:    botDisplayNames,
			OriginLocation:  "Internet",
			CurrentLocation: "Internet",
			AgeRange:        "N/A",
			Traits:          "Group Chat Bot",
			LastUpdated:     time.Now().UTC(),
			IsBot:           true,
			Username:        botInfo.Username,
		}
	}

	return &profile, nil
}
