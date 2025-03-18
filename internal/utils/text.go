// Package utils provides utility functions and components for MurailoBot,
// including scheduling, text processing, and other shared functionality.
package utils

import (
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"sync"
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
		tokenizer, tokenizerErr = tiktoken.GetEncoding("cl100k_base")
		if tokenizerErr != nil {
			slog.Error("failed to initialize tokenizer", "error", tokenizerErr)
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
	if input == "" {
		return "", errors.New("empty input string")
	}

	// Process text in sequence without logging each intermediate step
	// 1. Remove metadata prefix (timestamp and user identifier)
	input = metadataFormatRegex.ReplaceAllString(input, "")

	// 2. Normalize line endings and Unicode characters
	s := strings.ReplaceAll(input, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = unicodeReplacer.Replace(s)

	// 3. Replace control characters with spaces
	s = controlCharsRegex.ReplaceAllString(s, " ")

	// 4. Normalize whitespace in each line
	parts := strings.Split(s, "\n")
	for i := range parts {
		parts[i] = normalizeLineWhitespace(parts[i])
	}

	// 5. Finalize formatting
	s = strings.Join(parts, "\n")
	s = multipleNewlinesRegex.ReplaceAllString(s, "\n\n")
	result := strings.TrimSpace(s)

	if result == "" {
		return "", errors.New("sanitization resulted in empty string")
	}

	// Only log if significant reduction happened (useful for debugging)
	reduction := 1.0 - float64(len(result))/float64(len(input))
	if reduction > 0.2 { // Only log if more than 20% reduction
		slog.Debug("significant text reduction during sanitization",
			"reduction_percent", int(reduction*100))
	}

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
	// Add 20% overhead for safety margin
	const tokenOverheadFactor = 1.2

	tk, err := getTokenizer()
	if err != nil {
		// Fallback to character-based approximation if tokenizer fails
		estimate := int(float64(len(text)/3+5) * tokenOverheadFactor)

		// Only log at debug level since this warning could appear frequently
		slog.Debug("using approximate token estimation",
			"method", "character-based",
			"estimate", estimate)

		return estimate
	}

	tokens := tk.Encode(text, nil, nil)
	estimate := int(float64(len(tokens))*tokenOverheadFactor + 0.5)

	// No need to log routine token estimations
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
	// Calculate remaining token budget
	availableTokens := maxTokens - systemPromptTokens - currentMessageTokens

	if availableTokens <= 0 || len(messages) == 0 {
		return []*db.Message{}
	}

	usedTokens := 0
	lastIncludedIndex := len(messages)

	// Iterate backwards through messages (newest to oldest)
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := EstimateTokens(messages[i].Content)
		totalMsgTokens := msgTokens + 15 // Add 15 for metadata overhead

		if usedTokens+totalMsgTokens > availableTokens {
			lastIncludedIndex = i + 1
			break
		}

		usedTokens += totalMsgTokens
		lastIncludedIndex = i
	}

	// Get the selected messages
	selectedMessages := messages[lastIncludedIndex:]

	// Only log if a significant portion of messages was dropped
	if len(selectedMessages) < len(messages) {
		percentIncluded := float64(len(selectedMessages)) / float64(len(messages)) * 100
		slog.Debug("message context truncated",
			"included", len(selectedMessages),
			"total", len(messages),
			"percent_included", int(percentIncluded))
	}

	return selectedMessages
}
