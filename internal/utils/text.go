// Package utils provides utility functions and components for MurailoBot,
// including scheduling, text processing, and other shared functionality.
package utils

import (
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/edgard/murailobot/internal/db"
	"github.com/pkoukk/tiktoken-go"
)

var (
	controlCharsRegex     = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`)
	multipleNewlinesRegex = regexp.MustCompile(`\n{3,}`)
	metadataFormatRegex   = regexp.MustCompile(`^\s*\[\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})\]\s+[^:]*(?::[^:]*)*:\s*`)
	unicodeReplacer       = strings.NewReplacer(
		// Zero-width joiners and non-joiners - replace with space to preserve word boundaries
		"\u2060", "", // Word joiner - remove completely
		"\uFEFF", "", // Zero width no-break space (BOM) - remove completely
		"\u00AD", "", // Soft hyphen - remove completely
		"\u200E", "", // Left-to-right mark - remove completely
		"\u200F", "", // Right-to-left mark - remove completely
		"\u2061", "", // Function application - remove completely
		"\u2062", "", // Invisible times - remove completely
		"\u2063", "", // Invisible separator - remove completely
		"\u2064", "", // Invisible plus - remove completely
		"\u200B", " ", // Zero width space - replace with space
		"\u200C", " ", // Zero width non-joiner - replace with space

		// Line/paragraph separators - convert to newlines
		"\u2028", "\n", // Line separator
		"\u2029", "\n\n", // Paragraph separator

		// Various space characters - convert to regular space
		"\u205F", " ", // Medium mathematical space
		"\u2009", " ", // Thin space
		"\u200A", " ", // Hair space
		"\u202F", " ", // Narrow no-break space
		"\u3000", " ", // Ideographic space
		"\u00A0", " ", // No-break space
	)
)

var (
	tokenizer     *tiktoken.Tiktoken
	tokenizerOnce sync.Once
	tokenizerErr  error
)

func getTokenizer() (*tiktoken.Tiktoken, error) {
	tokenizerOnce.Do(func() {
		slog.Debug("initializing tokenizer", "encoding", "cl100k_base")
		startTime := time.Now()

		tokenizer, tokenizerErr = tiktoken.GetEncoding("cl100k_base")

		if tokenizerErr != nil {
			slog.Error("failed to initialize tokenizer",
				"error", tokenizerErr,
				"duration_ms", time.Since(startTime).Milliseconds())
		} else {
			slog.Debug("tokenizer initialized successfully",
				"duration_ms", time.Since(startTime).Milliseconds())
		}
	})

	return tokenizer, tokenizerErr
}

func normalizeLineWhitespace(line string) string {
	// This function collapses all consecutive whitespace characters into a single space
	// and trims leading/trailing whitespace, preserving the content while normalizing spacing
	var strBuilder strings.Builder
	var space bool

	for _, r := range line {
		if unicode.IsSpace(r) {
			if !space {
				strBuilder.WriteRune(' ')
				space = true
			}
		} else {
			strBuilder.WriteRune(r)
			space = false
		}
	}

	return strings.TrimSpace(strBuilder.String())
}

// Sanitize normalizes and cleans text by removing problematic characters,
// standardizing whitespace, and ensuring consistent formatting.
//
// It handles various Unicode characters, control characters, and
// normalizes line endings and whitespace.
//
// Returns an error if the input is empty or if sanitization results
// in an empty string.
func Sanitize(input string) (string, error) {
	startTime := time.Now()
	inputLength := len(input)

	slog.Debug("sanitizing text input",
		"input_length", inputLength)

	if input == "" {
		slog.Error("sanitization failed: empty input string")
		return "", errors.New("empty input string")
	}

	// First remove any metadata prefix (timestamp and user identifier)
	// that might be present in message content
	metadataStart := time.Now()
	input = metadataFormatRegex.ReplaceAllString(input, "")
	slog.Debug("metadata removal completed",
		"duration_ms", time.Since(metadataStart).Milliseconds())

	// Normalize all line endings to \n and handle special Unicode characters
	normalizationStart := time.Now()
	s := strings.ReplaceAll(input, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = unicodeReplacer.Replace(s)
	slog.Debug("line ending and unicode normalization completed",
		"duration_ms", time.Since(normalizationStart).Milliseconds())

	// Replace control characters with spaces to preserve word boundaries
	// This is important for cases like "Hello\0world" which should become "Hello world"
	controlStart := time.Now()
	s = controlCharsRegex.ReplaceAllString(s, " ")
	slog.Debug("control character replacement completed",
		"duration_ms", time.Since(controlStart).Milliseconds())

	// Process each line individually to normalize whitespace within lines
	whitespaceStart := time.Now()
	parts := strings.Split(s, "\n")
	for i := range parts {
		parts[i] = normalizeLineWhitespace(parts[i])
	}
	slog.Debug("whitespace normalization completed",
		"line_count", len(parts),
		"duration_ms", time.Since(whitespaceStart).Milliseconds())

	// Rejoin lines and normalize multiple consecutive newlines to at most two
	// (preserving paragraph breaks but removing excessive empty lines)
	finalizeStart := time.Now()
	s = strings.Join(parts, "\n")
	s = multipleNewlinesRegex.ReplaceAllString(s, "\n\n")
	slog.Debug("newline normalization completed",
		"duration_ms", time.Since(finalizeStart).Milliseconds())

	result := strings.TrimSpace(s)
	if result == "" {
		slog.Error("sanitization resulted in empty string",
			"original_length", inputLength,
			"duration_ms", time.Since(startTime).Milliseconds())
		return "", errors.New("sanitization resulted in empty string")
	}

	totalDuration := time.Since(startTime)
	slog.Debug("text sanitization completed successfully",
		"original_length", inputLength,
		"result_length", len(result),
		"reduction_percent", int((1.0-float64(len(result))/float64(inputLength))*100),
		"duration_ms", totalDuration.Milliseconds())

	return result, nil
}

// EstimateTokens estimates the number of tokens in a text string for
// AI model token counting purposes. It uses the tiktoken library for
// accurate token counting with GPT models, with a fallback to a simple
// character-based approximation if the tokenizer fails.
//
// The estimate includes a 20% safety margin to account for potential
// variations in tokenization.
func EstimateTokens(text string) int {
	startTime := time.Now()
	textLength := len(text)

	slog.Debug("estimating tokens for text",
		"text_length", textLength)

	// Add 20% overhead to account for potential variations in tokenization
	// and ensure we don't underestimate token counts for context management
	const tokenOverheadFactor = 1.2

	tk, err := getTokenizer()
	if err != nil {
		// Fallback to a simple character-based approximation if tokenizer fails:
		// Roughly 3 characters per token plus a small constant, with safety margin
		estimate := int(float64(len(text)/3+5) * tokenOverheadFactor)

		slog.Warn("falling back to approximate token estimation",
			"error", err,
			"text_length", textLength,
			"estimated_tokens", estimate,
			"duration_ms", time.Since(startTime).Milliseconds())

		return estimate
	}

	encodeStart := time.Now()
	tokens := tk.Encode(text, nil, nil)
	encodeDuration := time.Since(encodeStart)

	// Add 0.5 before casting to int for proper rounding
	estimate := int(float64(len(tokens))*tokenOverheadFactor + 0.5)

	slog.Debug("token estimation completed",
		"text_length", textLength,
		"raw_token_count", len(tokens),
		"estimated_tokens", estimate,
		"encode_duration_ms", encodeDuration.Milliseconds(),
		"total_duration_ms", time.Since(startTime).Milliseconds(),
		"chars_per_token", float64(textLength)/float64(len(tokens)))

	return estimate
}

// SelectMessages chooses a subset of messages that fit within a specified
// token budget, prioritizing the most recent messages. This is used to
// create a context window for AI models that have token limits.
//
// Parameters:
// - maxTokens: The maximum total tokens allowed
// - messages: The full list of messages to select from
// - systemPromptTokens: Tokens already used by the system prompt
// - currentMessageTokens: Tokens used by the current user message
//
// Returns a slice of messages that fit within the token budget.
func SelectMessages(
	maxTokens int,
	messages []*db.Message,
	systemPromptTokens int,
	currentMessageTokens int,
) []*db.Message {
	startTime := time.Now()

	slog.Debug("selecting messages for token budget",
		"max_tokens", maxTokens,
		"message_count", len(messages),
		"system_prompt_tokens", systemPromptTokens,
		"current_message_tokens", currentMessageTokens)

	// Calculate remaining token budget after accounting for system prompt and current message
	availableTokens := maxTokens - systemPromptTokens - currentMessageTokens

	if availableTokens <= 0 || len(messages) == 0 {
		slog.Debug("no tokens available or no messages to select",
			"available_tokens", availableTokens,
			"message_count", len(messages))
		return []*db.Message{}
	}

	usedTokens := 0
	lastIncludedIndex := len(messages)
	tokenEstimationStart := time.Now()

	// Track token usage per message for debugging
	messageTokens := make(map[uint]int)

	// Iterate backwards through messages (newest to oldest) to prioritize recent context
	// This ensures the most recent conversation history is included when token limits are reached
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := EstimateTokens(messages[i].Content)
		// Add 15 tokens to account for message metadata and formatting overhead
		totalMsgTokens := msgTokens + 15

		// Store token count for this message
		messageTokens[messages[i].ID] = totalMsgTokens

		if usedTokens+totalMsgTokens > availableTokens {
			slog.Debug("token budget exceeded, stopping message inclusion",
				"message_id", messages[i].ID,
				"message_tokens", totalMsgTokens,
				"used_tokens", usedTokens,
				"available_tokens", availableTokens)
			lastIncludedIndex = i + 1
			break
		}

		usedTokens += totalMsgTokens
		lastIncludedIndex = i
	}

	tokenEstimationDuration := time.Since(tokenEstimationStart)

	// Get the selected messages
	selectedMessages := messages[lastIncludedIndex:]

	totalDuration := time.Since(startTime)
	slog.Debug("message selection completed",
		"total_messages", len(messages),
		"selected_messages", len(selectedMessages),
		"used_tokens", usedTokens,
		"available_tokens", availableTokens,
		"token_estimation_duration_ms", tokenEstimationDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds())

	// Return the subset of messages that fit within the token budget
	// The slice starts from the oldest message we could include
	return selectedMessages
}
