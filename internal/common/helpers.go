package common

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	// Regular expressions for sanitization
	urlRegex        = regexp.MustCompile(`https?://[^\s]+`)
	mentionRegex    = regexp.MustCompile(`@\w+`)
	hashtagRegex    = regexp.MustCompile(`#\w+`)
	multiSpaceRegex = regexp.MustCompile(`\s+`)
)

// SanitizeText cleans text by:
// - Removing URLs
// - Removing mentions and hashtags
// - Removing excessive whitespace and control characters
// - Trimming leading/trailing whitespace
func SanitizeText(text string) string {
	// Replace URLs with a space
	text = urlRegex.ReplaceAllString(text, " ")

	// Replace mentions and hashtags with a space
	text = mentionRegex.ReplaceAllString(text, " ")
	text = hashtagRegex.ReplaceAllString(text, " ")

	// Remove control characters
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)

	// Replace multiple spaces with a single space and trim
	text = multiSpaceRegex.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	return text
}
