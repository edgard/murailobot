package utils

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Logging constants define the available logging levels and formats.
const (
	// Log level constants.
	logLevelDebug = "debug" // Debug level for detailed troubleshooting
	logLevelWarn  = "warn"  // Warning level for potential issues
	logLevelError = "error" // Error level for actual failures
	logLevelInfo  = "info"  // Info level for general operational events

	// Log format constants.
	logFormatText = "text" // Human-readable text format
	logFormatJSON = "json" // Machine-readable JSON format
)

// Text sanitization threshold constants define limits for text processing operations.
const (
	minNewlinesThreshold    = 3 // Minimum consecutive newlines to collapse
	minHorizontalRuleLength = 3 // Minimum length for horizontal rule detection
	minMarkdownLinkGroups   = 3 // Expected capture groups in markdown link regex
	markdownHeaderMinLevel  = 1 // Minimum header level (#)
	markdownHeaderMaxLevel  = 6 // Maximum header level (######)
)

// Error definitions for logging configuration.
var (
	ErrInvalidLogLevel  = errors.New("invalid log level")  // Unsupported log level
	ErrInvalidLogFormat = errors.New("invalid log format") // Unsupported log format
)

// Replacer groups define character substitution mappings for text processing.
var (
	// unicodeReplacer handles special Unicode control and formatting characters.
	unicodeReplacer = strings.NewReplacer(
		"\u2060", "", "\u180E", "", // Zero-width spaces
		"\u2028", "\n", "\u2029", "\n\n", // Line/paragraph separators
		"\u200B", " ", "\u200C", " ", // Zero-width characters
		"\u200D", "", "\uFEFF", "", // Zero-width joiners and BOM
		"\u00AD", "", "\u205F", " ", // Soft hyphen and math space
		"\u202A", "", "\u202B", "", // Directional formatting
		"\u202C", "", "\u202D", "", "\u202E", "", // More directional formatting
	)

	// escapedReplacer preserves escaped markdown symbols by converting
	// them to temporary Unicode characters.
	escapedReplacer = strings.NewReplacer(
		"\\*", "\u0001", "\\_", "\u0002",
		"\\`", "\u0003", "\\~", "\u0004",
		"\\[", "\u0005", "\\]", "\u0006",
		"\\(", "\u0007", "\\)", "\u0008",
		"\\#", "\u000E", "\\>", "\u000F",
		"\\!", "\u0010",
	)

	// restoreReplacer converts the temporary Unicode characters back
	// to their original markdown symbols.
	restoreReplacer = strings.NewReplacer(
		"\u0001", "*", "\u0002", "_",
		"\u0003", "`", "\u0004", "~",
		"\u0005", "[", "\u0006", "]",
		"\u0007", "(", "\u0008", ")",
		"\u000E", "#", "\u000F", ">", "\u0010", "!",
	)
)

// Regex pattern groups define regular expressions for matching various
// markdown and text formatting constructs.
var (
	// Basic character patterns.
	controlCharsRegex     = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`)
	multipleNewlinesRegex = regexp.MustCompile("\n{" + strconv.Itoa(minNewlinesThreshold) + ",}")
	horizontalRuleRegex   = regexp.MustCompile("^[\\*\\-_]{" + strconv.Itoa(minHorizontalRuleLength) + ",}$")

	// Code block patterns.
	fencedCodeBlocksRegex = regexp.MustCompile("```[\\s\\S]*?```")
	inlineCodeRegex       = regexp.MustCompile("`[^`]+`")

	// Link patterns.
	imagesRegex = regexp.MustCompile(`!\[(.*?)\]\(([^)]+)\)`)
	linksRegex  = regexp.MustCompile(`\[(.*?)\]\(([^)]+)\)`)

	// Text formatting patterns.
	headersRegex   = regexp.MustCompile("(?m)^#{" + strconv.Itoa(markdownHeaderMinLevel) + "," + strconv.Itoa(markdownHeaderMaxLevel) + "} (.+)$")
	boldRegex      = regexp.MustCompile(`\*\*(.*?)\*\*`)
	boldAltRegex   = regexp.MustCompile(`__(.+?)__`)
	italicRegex    = regexp.MustCompile(`\*([^*]+)\*`)
	italicAltRegex = regexp.MustCompile(`_([^_]+)_`)
	strikeRegex    = regexp.MustCompile(`~~(.+?)~~`)

	// List patterns.
	orderedListRegex  = regexp.MustCompile(`^\s*\d+\.\s+`)
	numberedListRegex = regexp.MustCompile(`^\d+\.\s+`)
	blockquotesRegex  = regexp.MustCompile(`(?m)^>\s*(.+)$`)

	// HTML and additional patterns.
	htmlTagsRegex = regexp.MustCompile(`<[^>]*>`)

	// markdownRegexps contains patterns used to detect markdown formatting.
	markdownRegexps = []*regexp.Regexp{
		regexp.MustCompile(`\*\*.+?\*\*`),                 // Bold with **
		regexp.MustCompile(`__.+?__`),                     // Bold with __
		regexp.MustCompile(`\*.+?\*`),                     // Italics with *
		regexp.MustCompile(`_.+?_`),                       // Italics with _
		regexp.MustCompile(`~~.+?~~`),                     // Strikethrough
		regexp.MustCompile(`\[.+?\]\(.+?\)`),              // Markdown links
		regexp.MustCompile(`!\[.+?\]\(.+?\)`),             // Markdown images
		regexp.MustCompile("```[\\s\\S]+?```"),            // Fenced code blocks
		regexp.MustCompile("`[^`]+`"),                     // Inline code
		regexp.MustCompile(`(?m)^#{1,6} .+$`),             // Headers
		regexp.MustCompile(`(?m)^> .+$`),                  // Blockquotes
		regexp.MustCompile(`(?m)^[\*\-\+] .+$`),           // Unordered lists
		regexp.MustCompile(`(?m)^\d+\. .+$`),              // Ordered lists
		regexp.MustCompile(`(?m)^[\*\-_]{3,}$`),           // Horizontal rules
		regexp.MustCompile(`(?m)^\|.+\|$`),                // Tables
		regexp.MustCompile(`(?m)^.+\r?\n(=+|-+)\s*$`),     // Setext headers
		regexp.MustCompile(`(?m)^[-*] \[(?: |x|X)\] .+$`), // Task lists
		regexp.MustCompile(`(?m)^\[\^.+\]:\s+.+$`),        // Footnotes
		regexp.MustCompile(`(?m)^( {4}|\t).+`),            // Indented code
		regexp.MustCompile(`(?m)\$[^$\n]+\$`),             // Inline math
		regexp.MustCompile(`(?m)\$\$[\s\S]+\$\$`),         // Display math
	}
)
