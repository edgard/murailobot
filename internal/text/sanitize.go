// Package text provides text sanitization.
package text

import (
	"strings"
	"unicode"

	errs "github.com/edgard/murailobot/internal/errors"
)

// normalizeLineWhitespace collapses all consecutive whitespace characters (spaces, tabs, etc.)
// into a single space and trims leading/trailing whitespace from a line of text.
//
// This is used as a final whitespace normalization step after Unicode character replacement.
// It's applied separately to each line of text to ensure consistent spacing within paragraphs
// while preserving paragraph breaks (newlines).
func normalizeLineWhitespace(line string) string {
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

// Sanitize processes text to create normalized, clean output by removing problematic
// characters and standardizing formatting. It performs the following operations in sequence:
//
// 1. Input validation - returns error for empty input
// 2. Metadata removal - strips timestamp prefixes like "[2025-03-06T22:30:11+01:00] USER:"
// 3. Line ending normalization - converts all line endings (CRLF, CR) to LF ('\n')
// 4. Unicode character handling - normalizes or removes special Unicode characters:
//   - Removes invisible format control characters
//   - Eliminates directional formatting marks
//   - Converts various Unicode spaces to standard spaces
//   - Standardizes line/paragraph separators
//
// 5. Control character removal - removes ASCII control characters for security
// 6. Whitespace normalization - collapses multiple spaces within each line
// 7. Line break normalization - converts excessive newlines (3+) to exactly two
// 8. Final whitespace trimming - removes leading/trailing whitespace
//
// Returns the sanitized string or an error if the input was empty or
// if sanitization resulted in an empty string.
func Sanitize(input string) (string, error) {
	if input == "" {
		return "", errs.NewValidationError("empty input string", nil)
	}

	input = MetadataFormatRegex.ReplaceAllString(input, "")

	s := strings.ReplaceAll(input, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = UnicodeReplacer.Replace(s)
	s = ControlCharsRegex.ReplaceAllString(s, " ")

	parts := strings.Split(s, "\n")
	for i := range parts {
		parts[i] = normalizeLineWhitespace(parts[i])
	}

	s = strings.Join(parts, "\n")
	s = MultipleNewlinesRegex.ReplaceAllString(s, "\n\n")

	result := strings.TrimSpace(s)
	if result == "" {
		return "", errs.NewValidationError("sanitization resulted in empty string", nil)
	}

	return result, nil
}
