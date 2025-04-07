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

// OpenAI implements the AI interface using OpenAI's API
type OpenAI struct {
	client    *openai.Client
	maxTokens int
	config    struct {
		token              string
		baseURL            string
		model              string
		maxTokens          int
		temperature        float32
		timeout            time.Duration
		instruction        string
		profileInstruction string
	}
}

// NewOpenAI creates a new OpenAI service instance
func NewOpenAI() (interfaces.AI, error) {
	return &OpenAI{}, nil
}

// Configure sets up the AI service with basic configuration
func (o *OpenAI) Configure(token, baseURL, model string, maxTokens int, temperature float32,
	timeout time.Duration, instruction, profileInstruction string,
) error {
	if token == "" {
		return common.ErrMissingToken
	}

	if model == "" {
		model = openai.GPT3Dot5Turbo
	}

	if temperature == 0 {
		temperature = 0.7
	}

	if timeout == 0 {
		timeout = 30 * time.Second
	}

	if maxTokens == 0 {
		maxTokens = 4000
	}

	aiConfig := openai.DefaultConfig(token)
	if baseURL != "" {
		aiConfig.BaseURL = baseURL
	}

	o.client = openai.NewClientWithConfig(aiConfig)
	o.maxTokens = maxTokens
	o.config.token = token
	o.config.baseURL = baseURL
	o.config.model = model
	o.config.maxTokens = maxTokens
	o.config.temperature = temperature
	o.config.timeout = timeout
	o.config.instruction = instruction
	o.config.profileInstruction = profileInstruction

	slog.Info("initializing OpenAI service", "model", model)
	return nil
}

// Stop gracefully shuts down the OpenAI service
func (o *OpenAI) Stop() error {
	slog.Info("stopping OpenAI service")
	return nil
}

// createSystemPrompt generates the system prompt for the AI model
func (o *OpenAI) createSystemPrompt(userProfiles map[int64]*models.UserProfile, botInfo models.BotInfo) string {
	displayName := botInfo.FirstName
	if displayName == "" {
		displayName = botInfo.UserName
	}

	botIdentityHeader := fmt.Sprintf(
		"You are %s, a Telegram bot in a group chat. When someone mentions you with @%s, "+
			"your task is to respond to their message. Messages will include the @%s mention - "+
			"this is normal and expected. Always respond directly to the content of the message. "+
			"Even if the message doesn't contain a clear question, assume it's directed at you "+
			"and respond appropriately.\n\n",
		displayName, botInfo.UserName, botInfo.UserName)

	systemPrompt := botIdentityHeader + o.config.instruction

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
			if id == botInfo.ID {
				// Inline the display name formatting logic
				botDisplayName := botInfo.UserName
				if botInfo.FirstName != "" && botInfo.FirstName != botInfo.UserName {
					botDisplayName = fmt.Sprintf("%s, %s", botInfo.FirstName, botInfo.UserName)
				}

				profileInfo.WriteString(fmt.Sprintf("UID %d (%s) | Internet | Internet | N/A | Group Chat Bot\n",
					id, botDisplayName))
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
func (o *OpenAI) GenerateResponse(ctx context.Context, messages []*models.Message, botInfo models.BotInfo) (string, error) {
	if len(messages) == 0 {
		return "", common.ErrEmptyInput
	}

	systemPrompt := o.createSystemPrompt(nil, botInfo)
	systemTokens, err := common.CountTokens(systemPrompt)
	if err != nil {
		return "", fmt.Errorf("%w: %v", common.ErrTokenCount, err)
	}

	availableTokens := o.maxTokens - systemTokens

	formattedMsgs := make([]string, len(messages))
	for i, msg := range messages {
		formattedMsgs[i] = fmt.Sprintf("[%s] UID %d: %s",
			msg.CreatedAt.Format(time.RFC3339),
			msg.UserID,
			msg.Content)
	}

	filteredMsgs, err := common.FilterContent(formattedMsgs, availableTokens, true)
	if err != nil {
		return "", fmt.Errorf("%w: %v", common.ErrTokenCount, err)
	}

	chatMessages := []openai.ChatCompletionMessage{{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	}}

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

	timeoutCtx, cancel := context.WithTimeout(ctx, o.config.timeout)
	defer cancel()

	resp, err := o.client.CreateChatCompletion(
		timeoutCtx,
		openai.ChatCompletionRequest{
			Model:       o.config.model,
			Messages:    chatMessages,
			Temperature: o.config.temperature,
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
func (o *OpenAI) GenerateProfile(ctx context.Context, userID int64, messages []*models.Message, botInfo models.BotInfo) (*models.UserProfile, error) {
	if len(messages) == 0 {
		return nil, common.ErrEmptyInput
	}

	if userID == botInfo.ID {
		// Inline the display name formatting logic
		displayNames := botInfo.UserName
		if botInfo.FirstName != "" && botInfo.FirstName != botInfo.UserName {
			displayNames = fmt.Sprintf("%s, %s", botInfo.FirstName, botInfo.UserName)
		}

		return &models.UserProfile{
			UserID:          botInfo.ID,
			DisplayNames:    displayNames,
			OriginLocation:  "Internet",
			CurrentLocation: "Internet",
			AgeRange:        "N/A",
			Traits:          "Group Chat Bot",
			LastUpdated:     time.Now().UTC(),
			IsBot:           true,
			Username:        botInfo.UserName,
		}, nil
	}

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

%s`, botInfo.ID, botInfo.UserName, botInfo.FirstName, o.config.profileInstruction)

	systemTokens, err := common.CountTokens(instruction)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", common.ErrTokenCount, err)
	}

	availableTokens := o.maxTokens - systemTokens
	formattedMsgs := make([]string, 0, len(messages))
	for _, msg := range messages {
		if msg.UserID == userID {
			content := fmt.Sprintf("[%s] %s\n",
				msg.CreatedAt.Format(time.RFC3339),
				msg.Content)
			formattedMsgs = append(formattedMsgs, content)
		}
	}

	filteredMsgs, err := common.FilterContent(formattedMsgs, availableTokens, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", common.ErrTokenCount, err)
	}

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

	timeoutCtx, cancel := context.WithTimeout(ctx, o.config.timeout)
	defer cancel()

	resp, err := o.client.CreateChatCompletion(
		timeoutCtx,
		openai.ChatCompletionRequest{
			Model:       o.config.model,
			Messages:    chatMessages,
			Temperature: o.config.temperature,
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
	if jsonStart == -1 || jsonEnd <= jsonStart {
		return nil, common.ErrInvalidJSON
	}

	var profile models.UserProfile
	if err := json.Unmarshal([]byte(response[jsonStart:jsonEnd+1]), &profile); err != nil {
		return nil, fmt.Errorf("%w: %v", common.ErrInvalidJSON, err)
	}

	profile.UserID = userID
	profile.LastUpdated = time.Now().UTC()

	return &profile, nil
}
