// Package util provides common utility functions for MurailoBot,
// including text processing and other shared functionality.
package util

import (
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"unicode"

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

// GetTokenizer returns the tiktoken tokenizer for direct token counting.
// This provides access to the raw tokenizer without any safety margins or estimates.
func GetTokenizer() (*tiktoken.Tiktoken, error) {
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

	return result, nil
}
