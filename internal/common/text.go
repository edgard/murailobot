package common

import (
	"regexp"
	"strings"
	"unicode"
)

// Matches multiple spaces or tabs (including Unicode whitespace)
var spaceRegex = regexp.MustCompile(`[ \t\p{Zs}]+`)

// NormalizeText standardizes text by:
// - Converting all newline variants (\r\n, \r) to \n
// - Replacing multiple spaces/tabs/unicode whitespace with a single space
// - Ensuring a maximum of two consecutive newlines
// - Collapsing whitespace-only lines
// - Trimming leading/trailing whitespace from each line and final result
// - Preserving emojis and other special characters
func NormalizeText(text string) string {
	if text == "" {
		return text
	}

	// Replace tabs with spaces to ensure consistent handling
	text = strings.ReplaceAll(text, "\t", " ")

	// Remove control characters (except newlines which are handled separately)
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\r' {
			return -1 // Remove the character
		}
		return r
	}, text)

	// Normalize all newlines to \n
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	// Process lines
	var builder strings.Builder
	var prevLineEmpty bool

	for i, line := range strings.Split(text, "\n") {
		// Replace multiple spaces with a single space and trim
		line = spaceRegex.ReplaceAllString(line, " ")
		line = strings.TrimSpace(line)

		// Skip empty lines if previous line was also empty
		if line == "" {
			if prevLineEmpty {
				continue
			}
			prevLineEmpty = true
		} else {
			prevLineEmpty = false
		}

		// Add newline before line (except for the first line)
		if i > 0 {
			builder.WriteByte('\n')
		}

		builder.WriteString(line)
	}

	// Trim leading/trailing whitespace from the result
	return strings.TrimSpace(builder.String())
}
