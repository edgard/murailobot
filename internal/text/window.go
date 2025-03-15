package text

import (
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/logging"
)

// DynamicWindow manages message selection based on token limits
type DynamicWindow struct {
	MaxTokens int // Maximum tokens for context window
}

// NewDynamicWindow creates a new dynamic window manager
func NewDynamicWindow(maxTokens int) *DynamicWindow {
	return &DynamicWindow{
		MaxTokens: maxTokens,
	}
}

// EstimateTokens provides a simple ballpark token count
// that works reasonably well across different models
func EstimateTokens(text string) int {
	// Simple approximation that works fairly well across most models
	// Character count divided by 3 tends to be a decent middle ground
	// between different tokenization schemes
	return len(text)/3 + 5 // Add a small buffer for safety
}

// SelectMessages chooses messages to fit within token budget
func (dw *DynamicWindow) SelectMessages(
	messages []db.GroupMessage,
	systemPromptTokens int,
	currentMessageTokens int,
) []db.GroupMessage {
	// Calculate available tokens
	availableTokens := dw.MaxTokens - systemPromptTokens - currentMessageTokens

	if availableTokens <= 0 || len(messages) == 0 {
		return []db.GroupMessage{}
	}

	// Count backward from most recent messages
	// until we reach our token budget
	usedTokens := 0
	lastIncludedIndex := len(messages)

	for i := len(messages) - 1; i >= 0; i-- {
		// Estimate message tokens
		msgTokens := EstimateTokens(messages[i].Message)

		// Add overhead for message formatting (timestamp, user ID, etc.)
		// This is typically around 15 tokens
		totalMsgTokens := msgTokens + 15

		// If adding this message would exceed our budget, stop
		if usedTokens+totalMsgTokens > availableTokens {
			lastIncludedIndex = i + 1
			break
		}

		// Otherwise, include it
		usedTokens += totalMsgTokens
		lastIncludedIndex = i
	}

	// Return the selected messages in chronological order
	selected := messages[lastIncludedIndex:]

	logging.Info("dynamic context selection",
		"total_messages", len(messages),
		"selected_messages", len(selected),
		"estimated_tokens", usedTokens,
		"available_tokens", availableTokens)

	return selected
}
