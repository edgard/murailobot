// Package ai provides interfaces and implementations for interacting with different AI backends.
package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/db"
)

// AICore holds shared configuration and logic applicable across different AI backends.
type AICore struct {
	storage            *db.DB
	botInfo            BotInfo
	instruction        string
	profileInstruction string
	timeout            time.Duration
	temperature        float32 // Keep temperature here if it's conceptually shared, or move to specific clients if needed.
}

// NewAICore creates a new instance of AICore.
func NewAICore(storage *db.DB, botInfo BotInfo, instruction, profileInstruction string, timeout time.Duration, temperature float32) (*AICore, error) {
	if storage == nil {
		return nil, errors.New("nil storage provided to AICore")
	}
	// Validation removed here - BotInfo will be set later via SetBotInfo.
	// if botInfo.UserID <= 0 || botInfo.Username == "" {
	// return nil, errors.New("invalid bot info provided to AICore")
	// }
	return &AICore{
		storage:            storage,
		botInfo:            botInfo,
		instruction:        instruction,
		profileInstruction: profileInstruction,
		timeout:            timeout,
		temperature:        temperature,
	}, nil
}

// SetBotInfo updates the bot's identity information.
func (c *AICore) SetBotInfo(info BotInfo) error {
	if info.UserID <= 0 {
		return errors.New("invalid bot user ID")
	}
	if info.Username == "" {
		return errors.New("empty bot username")
	}
	c.botInfo = info
	return nil
}

// BotInfo returns the current bot information.
func (c *AICore) BotInfo() BotInfo {
	return c.botInfo
}

// Timeout returns the configured AI timeout.
func (c *AICore) Timeout() time.Duration {
	return c.timeout
}

// Temperature returns the configured AI temperature.
func (c *AICore) Temperature() float32 {
	return c.temperature
}

// formatMessage formats a database message into a string representation for the AI.
func formatMessage(msg *db.Message) string {
	return fmt.Sprintf("[%s] UID %d: %s",
		msg.Timestamp.Format(time.RFC3339),
		msg.UserID,
		msg.Content)
}

// CreateSystemPrompt generates the system prompt for the AI model based on the bot's identity
// and user profiles. This prompt helps the AI understand its role in the conversation.
func (c *AICore) CreateSystemPrompt(userProfiles map[int64]*db.UserProfile) string {
	// Add check for valid BotInfo before using it
	if c.botInfo.UserID <= 0 {
		slog.Error("AICore.CreateSystemPrompt called before valid BotInfo was set")
		// Return base instruction without bot details
		return c.instruction
	}

	// Use the bot's display name if available, otherwise fall back to username
	displayName := c.botInfo.Username
	if c.botInfo.DisplayName != "" {
		displayName = c.botInfo.DisplayName
	}

	// Create a personalized header that defines the bot's identity and expected behavior
	botIdentityHeader := fmt.Sprintf(
		"You are %s, a Telegram bot in a group chat. When someone mentions you with @%s, "+
			"your task is to respond to their message. Messages will include the @%s mention - "+
			"this is normal and expected. Always respond directly to the content of the message. "+
			"Even if the message doesn't contain a clear question, assume it's directed at you "+
			"and respond appropriately.\n\n",
		displayName, c.botInfo.Username, c.botInfo.Username)

	// Combine the identity header with the configured instruction text
	systemPrompt := botIdentityHeader + c.instruction

	// If user profiles are available, append them to provide context about the participants
	if len(userProfiles) > 0 {
		var profileInfo strings.Builder

		profileInfo.WriteString("\n\n## USER PROFILES\n")
		profileInfo.WriteString("Format: UID [user_id] ([display_names]) | [origin_location] | [current_location] | [age_range] | [traits]\n\n")

		userIDs := make([]int64, 0, len(userProfiles))
		for id := range userProfiles {
			userIDs = append(userIDs, id)
		}
		sort.Slice(userIDs, func(i, j int) bool { return userIDs[i] < userIDs[j] })

		for _, id := range userIDs {
			if id == c.botInfo.UserID {
				botDisplayNames := c.botInfo.Username
				if c.botInfo.DisplayName != "" && c.botInfo.DisplayName != c.botInfo.Username {
					botDisplayNames = fmt.Sprintf("%s, %s", c.botInfo.DisplayName, c.botInfo.Username)
				}
				profileInfo.WriteString(fmt.Sprintf("UID %d (%s) | Internet | Internet | N/A | Group Chat Bot\n", id, botDisplayNames))
				continue
			}
			profile := userProfiles[id]
			profileInfo.WriteString(profile.FormatPipeDelimited() + "\n")
		}
		systemPrompt += profileInfo.String()
	}

	return systemPrompt
}

// getProfileInstruction generates the detailed instruction prompt for the profile generation task.
func (c *AICore) getProfileInstruction() string {
	// Add check for valid BotInfo before using it
	if c.botInfo.UserID <= 0 {
		slog.Error("AICore.getProfileInstruction called before valid BotInfo was set")
		// Return base instruction without bot details - this might cause issues for profile generation
		return c.profileInstruction
	}

	slog.Debug("generating profile instruction",
		"bot_id", c.botInfo.UserID,
		"bot_username", c.botInfo.Username)
	startTime := time.Now()

	botIdentificationAndFixedPart := fmt.Sprintf(`
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
Return ONLY a JSON object, no additional text, with this structure:
{
  "users": {
    "[user_id]": {
      "display_names": "Comma-separated list of names/nicknames",
      "origin_location": "Where the user is from",
      "current_location": "Where the user currently lives",
      "age_range": "Approximate age range (20s, 30s, etc.)",
      "traits": "Comma-separated list of personality traits and characteristics"
    }
  }
}`, c.botInfo.UserID, c.botInfo.Username, c.botInfo.DisplayName)

	fullInstruction := fmt.Sprintf("%s\n\n%s", c.profileInstruction, botIdentificationAndFixedPart)

	instructionLength := len(fullInstruction)
	configLength := len(c.profileInstruction)
	fixedPartLength := len(botIdentificationAndFixedPart)
	duration := time.Since(startTime)
	slog.Debug("profile instruction generated",
		"total_length", instructionLength,
		"config_length", configLength,
		"fixed_part_length", fixedPartLength,
		"duration_ms", duration.Milliseconds())

	return fullInstruction
}

// parseProfileResponse parses the JSON response from the AI during profile generation.
func (c *AICore) parseProfileResponse(response string, userMessages map[int64][]*db.Message, existingProfiles map[int64]*db.UserProfile) (map[int64]*db.UserProfile, error) {
	startTime := time.Now()
	slog.Debug("parsing profile response", "response_length", len(response))

	response = strings.TrimSpace(response)
	if response == "" {
		slog.Error("empty profile response received")
		return nil, errors.New("empty profile response")
	}

	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		slog.Error("invalid JSON format in response", "json_start", jsonStart, "json_end", jsonEnd)
		return nil, errors.New("invalid JSON format in response")
	}
	jsonContent := response[jsonStart : jsonEnd+1]
	slog.Debug("extracted JSON content", "json_length", len(jsonContent))

	var profileResponse struct {
		Users map[string]struct {
			DisplayNames    string `json:"display_names"`
			OriginLocation  string `json:"origin_location"`
			CurrentLocation string `json:"current_location"`
			AgeRange        string `json:"age_range"`
			Traits          string `json:"traits"`
		} `json:"users"`
	}

	unmarshalStartTime := time.Now()
	err := json.Unmarshal([]byte(jsonContent), &profileResponse)
	unmarshalDuration := time.Since(unmarshalStartTime)
	if err != nil {
		slog.Error("failed to parse JSON response", "error", err, "unmarshal_duration_ms", unmarshalDuration.Milliseconds())
		return nil, fmt.Errorf("failed to parse profile response: %w", err)
	}
	slog.Debug("JSON unmarshaled successfully", "duration_ms", unmarshalDuration.Milliseconds())

	if profileResponse.Users == nil {
		slog.Warn("no users found in profile response, initializing empty map")
		profileResponse.Users = make(map[string]struct {
			DisplayNames    string `json:"display_names"`
			OriginLocation  string `json:"origin_location"`
			CurrentLocation string `json:"current_location"`
			AgeRange        string `json:"age_range"`
			Traits          string `json:"traits"`
		})
	}

	updatedProfiles := make(map[int64]*db.UserProfile)
	newProfiles := 0
	updatedExistingProfiles := 0
	skippedProfiles := 0

	for userIDStr, profile := range profileResponse.Users {
		var userID int64
		if _, err := fmt.Sscanf(userIDStr, "%d", &userID); err != nil || userID == 0 {
			slog.Warn("invalid user ID in profile response", "user_id", userIDStr)
			skippedProfiles++
			continue
		}

		if userID == c.botInfo.UserID {
			slog.Debug("handling bot's own profile", "bot_id", c.botInfo.UserID)
			botDisplayNames := c.botInfo.Username
			if c.botInfo.DisplayName != "" && c.botInfo.DisplayName != c.botInfo.Username {
				botDisplayNames = fmt.Sprintf("%s, %s", c.botInfo.DisplayName, c.botInfo.Username)
			}
			updatedProfiles[userID] = &db.UserProfile{
				UserID:          userID,
				DisplayNames:    botDisplayNames,
				OriginLocation:  "Internet",
				CurrentLocation: "Internet",
				AgeRange:        "N/A",
				Traits:          "Group Chat Bot",
				LastUpdated:     time.Now().UTC(),
			}
			continue
		}

		if _, hasMessages := userMessages[userID]; !hasMessages {
			if _, hasProfile := existingProfiles[userID]; !hasProfile {
				slog.Debug("skipping user with no messages and no existing profile", "user_id", userID)
				skippedProfiles++
				continue
			}
		}

		_, isExisting := existingProfiles[userID]
		updatedProfiles[userID] = &db.UserProfile{
			UserID:          userID,
			DisplayNames:    profile.DisplayNames,
			OriginLocation:  profile.OriginLocation,
			CurrentLocation: profile.CurrentLocation,
			AgeRange:        profile.AgeRange,
			Traits:          profile.Traits,
			LastUpdated:     time.Now().UTC(),
		}

		if isExisting {
			updatedExistingProfiles++
		} else {
			newProfiles++
		}
		slog.Debug("profile created", "user_id", userID, "is_update", isExisting, "display_names", profile.DisplayNames)
	}

	slog.Debug("profile parsing completed", "total_profiles", len(updatedProfiles), "duration_ms", time.Since(startTime).Milliseconds())
	return updatedProfiles, nil
}
