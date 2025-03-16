// Package utils provides utility functions and components for MurailoBot,
// including scheduling, text processing, and other shared functionality.
package utils

import (
	"errors"
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

	// First remove any metadata prefix (timestamp and user identifier)
	// that might be present in message content
	input = metadataFormatRegex.ReplaceAllString(input, "")

	// Normalize all line endings to \n and handle special Unicode characters
	s := strings.ReplaceAll(input, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = unicodeReplacer.Replace(s)

	// Replace control characters with spaces to preserve word boundaries
	// This is important for cases like "Hello\0world" which should become "Hello world"
	s = controlCharsRegex.ReplaceAllString(s, " ")

	// Process each line individually to normalize whitespace within lines
	parts := strings.Split(s, "\n")
	for i := range parts {
		parts[i] = normalizeLineWhitespace(parts[i])
	}

	// Rejoin lines and normalize multiple consecutive newlines to at most two
	// (preserving paragraph breaks but removing excessive empty lines)
	s = strings.Join(parts, "\n")
	s = multipleNewlinesRegex.ReplaceAllString(s, "\n\n")

	result := strings.TrimSpace(s)
	if result == "" {
		return "", errors.New("sanitization resulted in empty string")
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
	// Add 20% overhead to account for potential variations in tokenization
	// and ensure we don't underestimate token counts for context management
	const tokenOverheadFactor = 1.2

	tk, err := getTokenizer()
	if err != nil {
		// Fallback to a simple character-based approximation if tokenizer fails:
		// Roughly 3 characters per token plus a small constant, with safety margin
		return int(float64(len(text)/3+5) * tokenOverheadFactor)
	}

	tokens := tk.Encode(text, nil, nil)
	// Add 0.5 before casting to int for proper rounding
	return int(float64(len(tokens))*tokenOverheadFactor + 0.5)
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
	// Calculate remaining token budget after accounting for system prompt and current message
	availableTokens := maxTokens - systemPromptTokens - currentMessageTokens

	if availableTokens <= 0 || len(messages) == 0 {
		return []*db.Message{}
	}

	usedTokens := 0
	lastIncludedIndex := len(messages)

	// Iterate backwards through messages (newest to oldest) to prioritize recent context
	// This ensures the most recent conversation history is included when token limits are reached
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := EstimateTokens(messages[i].Content)
		// Add 15 tokens to account for message metadata and formatting overhead
		totalMsgTokens := msgTokens + 15

		if usedTokens+totalMsgTokens > availableTokens {
			lastIncludedIndex = i + 1
			break
		}

		usedTokens += totalMsgTokens
		lastIncludedIndex = i
	}

	// Return the subset of messages that fit within the token budget
	// The slice starts from the oldest message we could include
	return messages[lastIncludedIndex:]
}
