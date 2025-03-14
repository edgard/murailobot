// Package text provides text sanitization utilities for cleaning and normalizing text content.
// It handles control characters, Unicode spaces, line endings, and other text formatting issues
// to ensure consistent and safe text output.
package text

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	minNewlinesThreshold = 3
)

// Regular expression patterns and character replacers used for text sanitization.
var (
	// controlCharsRegex matches ASCII control characters (including DEL 0x7F) that should be removed.
	controlCharsRegex = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`)

	// multipleNewlinesRegex matches sequences of 3 or more newlines.
	// Used to normalize excessive line breaks into a consistent double newline format
	// to maintain paragraph separation while removing unnecessary extra blank lines.
	multipleNewlinesRegex = regexp.MustCompile("\n{" + strconv.Itoa(minNewlinesThreshold) + ",}")

	// metadataFormatRegex matches timestamp metadata prefixes in the format:
	// "[2025-03-06T22:30:11+01:00] USER:" or with fractional seconds or UTC 'Z' timezone.
	// Used to remove bot message metadata prefixes that may appear in logs and automated messages.
	// Handles multiple colons in identifiers like "System:Log:Info:" and ensures only
	// removing metadata prefixes at the start of the content.
	metadataFormatRegex = regexp.MustCompile(`^\s*\[\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})\]\s+[^:]*(?::[^:]*)*:\s*`)

	// unicodeReplacer defines mappings for Unicode character normalization to ensure consistent
	// text formatting by handling special Unicode characters that may cause display issues.
	unicodeReplacer = strings.NewReplacer(
		// Invisible format control characters - remove these
		"\u2060", "", // Word Joiner - remove (invisible character used to prevent line breaks)
		"\uFEFF", "", // Byte Order Mark - remove (can cause issues at start of text)
		"\u00AD", "", // Soft Hyphen - remove (invisible until line break needed)

		// Directional formatting characters - normalize to nothing
		"\u200E", "", // Left-to-Right Mark - remove (invisible direction control)
		"\u200F", "", // Right-to-Left Mark - remove (invisible direction control)

		// Invisible mathematical notation - remove these
		"\u2061", "", // Function Application - remove (invisible math operator)
		"\u2062", "", // Invisible Times - remove (invisible math operator)
		"\u2063", "", // Invisible Separator - remove (invisible math operator)
		"\u2064", "", // Invisible Plus - remove (invisible math operator)

		// Whitespace normalization - convert to regular spaces/newlines
		"\u2028", "\n", // Line Separator - convert to regular newline
		"\u2029", "\n\n", // Paragraph Separator - convert to double newline
		"\u200B", " ", // Zero Width Space - convert to visible space
		"\u200C", " ", // Zero Width Non-Joiner - convert to visible space
		"\u205F", " ", // Medium Mathematical Space - convert to regular space
		"\u2009", " ", // Thin Space - convert to regular space
		"\u200A", " ", // Hair Space - convert to regular space
		"\u202F", " ", // Narrow No-Break Space - convert to regular space
		"\u3000", " ", // Ideographic Space - convert to regular space (common in CJK text)
		"\u00A0", " ", // Non-breaking Space - convert to regular space
	)
)
