// Package text provides text sanitization.
package text

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/edgard/murailobot/internal/errs"
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

	input = metadataFormatRegex.ReplaceAllString(input, "")

	s := strings.ReplaceAll(input, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = unicodeReplacer.Replace(s)
	s = controlCharsRegex.ReplaceAllString(s, " ")

	parts := strings.Split(s, "\n")
	for i := range parts {
		parts[i] = normalizeLineWhitespace(parts[i])
	}

	s = strings.Join(parts, "\n")
	s = multipleNewlinesRegex.ReplaceAllString(s, "\n\n")

	result := strings.TrimSpace(s)
	if result == "" {
		return "", errs.NewValidationError("sanitization resulted in empty string", nil)
	}

	return result, nil
}

// SanitizeJSON tries to clean invalid JSON that might come from AI responses.
// This handles common issues like trailing commas, unquoted keys, and comments.
func SanitizeJSON(input string) string {
	// Remove any Javascript-style comments
	commentRegex := regexp.MustCompile(`(/\*([^*]|[\r\n]|(\*+([^*/]|[\r\n])))*\*+/)|(//.*)`)
	noComments := commentRegex.ReplaceAllString(input, "")

	// Remove trailing commas in objects and arrays
	trailingCommaRegex := regexp.MustCompile(`,(\s*[\]}])`)
	noTrailingCommas := trailingCommaRegex.ReplaceAllString(noComments, "$1")

	// Ensure property names are double-quoted
	// This is a simplified version - a real implementation might need a parser
	unquotedKeyRegex := regexp.MustCompile(`([{,]\s*)([a-zA-Z_][a-zA-Z0-9_]*)(\s*:)`)
	quotedKeys := unquotedKeyRegex.ReplaceAllString(noTrailingCommas, `$1"$2"$3`)

	// Replace single quotes with double quotes for property values
	singleQuoteRegex := regexp.MustCompile(`:\s*'([^']*)'`)
	doubleQuotes := singleQuoteRegex.ReplaceAllString(quotedKeys, `: "$1"`)

	// Handle escaped quotes
	escapedQuoteRegex := regexp.MustCompile(`\\(['"])`)
	cleanedJSON := escapedQuoteRegex.ReplaceAllString(doubleQuotes, "$1")

	return cleanedJSON
}
