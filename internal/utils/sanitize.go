package utils

import (
	"regexp"
	"strings"
	"unicode"
)

// Sanitize cleans up text by normalizing whitespace, newlines,
// and removing various control characters. If the input appears to be markdown,
// it will be converted to plaintext first.
func Sanitize(input string) string {
	if input == "" {
		return ""
	}

	// Check if input is likely markdown and strip markdown formatting if needed
	if IsMarkdown(input) {
		input = StripMarkdown(input)
	}

	// Step 1: Handle newlines and line endings first to preserve them
	result := strings.ReplaceAll(input, "\r\n", "\n") // Normalize CRLF to LF
	result = strings.ReplaceAll(result, "\r", "\n")   // Convert CR to newline (standard approach)

	// Step 2: Replace specific unicode characters that need special handling
	// Characters that should be completely removed without leaving spaces
	result = strings.ReplaceAll(result, "\u2060", "") // Word joiner - remove entirely
	result = strings.ReplaceAll(result, "\u180E", "") // Mongolian vowel separator - remove entirely (not space)

	// Step 3: Replace special Unicode whitespace/control characters
	result = strings.ReplaceAll(result, "\u2028", "\n")   // Line separator
	result = strings.ReplaceAll(result, "\u2029", "\n\n") // Paragraph separator
	result = strings.ReplaceAll(result, "\u200B", " ")    // Zero-width space
	result = strings.ReplaceAll(result, "\u200C", " ")    // Zero-width non-joiner
	result = strings.ReplaceAll(result, "\u200D", "")     // Zero-width joiner
	result = strings.ReplaceAll(result, "\uFEFF", "")     // Byte order mark
	result = strings.ReplaceAll(result, "\u00AD", "")     // Soft hyphen
	result = strings.ReplaceAll(result, "\u205F", " ")    // Medium mathematical space

	// Remove bidirectional control characters
	result = strings.ReplaceAll(result, "\u202A", "") // LRE
	result = strings.ReplaceAll(result, "\u202B", "") // RLE
	result = strings.ReplaceAll(result, "\u202C", "") // PDF
	result = strings.ReplaceAll(result, "\u202D", "") // LRO
	result = strings.ReplaceAll(result, "\u202E", "") // RLO

	// Step 4: Replace control characters with spaces
	controlChars := regexp.MustCompile("[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]")
	result = controlChars.ReplaceAllString(result, " ")

	// Step 5: Normalize whitespace line by line while preserving newlines
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		lines[i] = normalizeLineWhitespace(line)
	}
	result = strings.Join(lines, "\n")

	// Step 6: Normalize multiple consecutive newlines to maximum of two
	multipleNewlines := regexp.MustCompile("\n{3,}")
	result = multipleNewlines.ReplaceAllString(result, "\n\n")

	// Step 7: Trim the entire result
	result = strings.TrimSpace(result)

	return result
}

// normalizeLineWhitespace collapses multiple whitespace characters into a single space
// and trims the line, but preserves specific fullwidth spaces
func normalizeLineWhitespace(line string) string {
	var builder strings.Builder
	prevWasSpace := false

	for _, r := range line {
		if r == '\u3000' { // Preserve fullwidth space
			builder.WriteRune(r)
			prevWasSpace = false
		} else if unicode.IsSpace(r) || r == '\u00A0' {
			// For regular spaces, non-breaking space
			if !prevWasSpace {
				builder.WriteRune(' ')
				prevWasSpace = true
			}
		} else {
			builder.WriteRune(r)
			prevWasSpace = false
		}
	}

	return strings.TrimSpace(builder.String())
}

// IsMarkdown checks if the input string likely contains Markdown formatting
func IsMarkdown(text string) bool {
	// Common markdown patterns to check - simplified for better matching
	markdownPatterns := []string{
		`\*\*.+?\*\*`,       // Bold: **text**
		`__.+?__`,           // Bold: __text__
		`\*.+?\*`,           // Italic: *text*
		`_.+?_`,             // Italic: _text_
		`~~.+?~~`,           // Strikethrough: ~~text~~
		`\[.+?\]\(.+?\)`,    // Links: [text](url)
		`!\[.+?\]\(.+?\)`,   // Images: ![alt](url)
		"```[\\s\\S]+?```",  // Code blocks
		"`[^`]+`",           // Inline code
		`(?m)^#{1,6} .+$`,   // Headers: # Heading
		`(?m)^> .+`,         // Blockquotes: > quote
		`(?m)^[\*\-\+] .+$`, // Unordered lists: * item, - item, + item
		`(?m)^\d+\. .+$`,    // Ordered lists: 1. item
		`(?m)^[\*\-_]{3,}$`, // Horizontal rules: ***, ---, ___
		`(?m)^\|.+\|$`,      // Table rows
	}

	for _, pattern := range markdownPatterns {
		re := regexp.MustCompile(pattern)
		if re.MatchString(text) {
			// Don't count escaped markdown as markdown
			if strings.Contains(pattern, `\*`) && strings.Contains(text, `\*`) {
				// Check if all asterisks are escaped
				escaped := regexp.MustCompile(`\\[\*]`)
				unescaped := regexp.MustCompile(`[^\\]\*`)
				if escaped.FindAllString(text, -1) != nil && unescaped.FindAllString(text, -1) == nil {
					continue
				}
			}
			return true
		}
	}

	return false
}

// StripMarkdown converts markdown text to plain text
func StripMarkdown(markdown string) string {
	// First handle escaped markdown characters
	escapedChars := map[string]string{
		"\\*": "\u0001", // Use Unicode control chars as temporary placeholders
		"\\_": "\u0002",
		"\\`": "\u0003",
		"\\~": "\u0004",
		"\\[": "\u0005",
		"\\]": "\u0006",
		"\\(": "\u0007",
		"\\)": "\u0008",
		"\\#": "\u000E",
		"\\>": "\u000F",
		"\\!": "\u0010",
	}

	for escaped, placeholder := range escapedChars {
		markdown = strings.ReplaceAll(markdown, escaped, placeholder)
	}

	// Completely remove code blocks but preserve paragraph structure
	markdown = regexp.MustCompile("```[\\s\\S]*?```").ReplaceAllString(markdown, "\n") // Replace fenced code blocks with newline
	markdown = regexp.MustCompile("`[^`]+`").ReplaceAllString(markdown, "")            // Remove inline code

	// Remove images ![alt](url) -> alt
	imageRegex := regexp.MustCompile(`!\[(.*?)\]\([^)]+\)`)
	markdown = imageRegex.ReplaceAllString(markdown, "$1")

	// Replace links [text](url) -> text
	linkRegex := regexp.MustCompile(`\[(.*?)\]\([^)]+\)`)
	markdown = linkRegex.ReplaceAllString(markdown, "$1")

	// Remove headers
	headerRegex := regexp.MustCompile(`(?m)^#{1,6} (.+)$`)
	markdown = headerRegex.ReplaceAllString(markdown, "$1")

	// Remove bold/italic/strikethrough markers
	markdown = regexp.MustCompile(`\*\*(.*?)\*\*`).ReplaceAllString(markdown, "$1") // **bold**
	markdown = regexp.MustCompile(`__(.+?)__`).ReplaceAllString(markdown, "$1")     // __bold__
	markdown = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(markdown, "$1")   // *italic*
	markdown = regexp.MustCompile(`_([^_]+)_`).ReplaceAllString(markdown, "$1")     // _italic_
	markdown = regexp.MustCompile(`~~(.+?)~~`).ReplaceAllString(markdown, "$1")     // ~~strike~~

	// Remove blockquotes
	blockquoteRegex := regexp.MustCompile(`(?m)^>\s*(.+)$`)
	markdown = blockquoteRegex.ReplaceAllString(markdown, "$1")

	// Precompile regular expressions used in loops
	orderedListRegex := regexp.MustCompile(`^\s*\d+\.\s+`)
	numberedListPrefixRegex := regexp.MustCompile(`^\d+\.\s+`)
	horizontalRuleRegex := regexp.MustCompile(`^[\*\-_]{3,}$`)
	htmlTagsRegex := regexp.MustCompile(`<[^>]*>`)
	multipleNewlinesRegex := regexp.MustCompile(`\n{3,}`)

	// Remove list markers while preserving structure
	lines := strings.Split(markdown, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "* ") ||
			strings.HasPrefix(strings.TrimSpace(line), "- ") ||
			strings.HasPrefix(strings.TrimSpace(line), "+ ") {
			trimmed := strings.TrimSpace(line)
			indent := line[:len(line)-len(trimmed)]
			lines[i] = indent + strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(trimmed, "* "), "- "), "+ "))
		} else if matched := orderedListRegex.MatchString(line); matched {
			trimmed := strings.TrimSpace(line)
			indent := line[:len(line)-len(trimmed)]
			lines[i] = indent + numberedListPrefixRegex.ReplaceAllString(trimmed, "")
		}
	}
	markdown = strings.Join(lines, "\n")

	// Process table rows
	lines = strings.Split(markdown, "\n")
	processedLines := make([]string, 0, len(lines))

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Check if it's a table row
		if strings.HasPrefix(line, "|") && strings.HasSuffix(line, "|") {
			// Check if it's a separator row
			if i+1 < len(lines) &&
				strings.HasPrefix(lines[i+1], "|") &&
				strings.HasSuffix(lines[i+1], "|") &&
				strings.Contains(lines[i+1], "---") {
				// This is a table header row, add it but skip the next separator row
				processedLines = append(processedLines, strings.TrimSpace(strings.ReplaceAll(strings.Trim(line, "|"), "|", " ")))
				i++ // Skip separator row
				continue
			} else if strings.Contains(line, "---") &&
				i > 0 &&
				strings.HasPrefix(lines[i-1], "|") {
				// This is a separator row itself, skip it
				continue
			} else {
				// Normal table row
				processedLines = append(processedLines, strings.TrimSpace(strings.ReplaceAll(strings.Trim(line, "|"), "|", " ")))
				continue
			}
		}

		// Check for horizontal rule
		if horizontalRuleRegex.MatchString(strings.TrimSpace(line)) {
			continue // Skip horizontal rules entirely
		}

		// Add all other lines
		processedLines = append(processedLines, line)
	}

	markdown = strings.Join(processedLines, "\n")

	// Remove HTML tags
	markdown = htmlTagsRegex.ReplaceAllString(markdown, "")

	// Fix multiple newlines that could result from removing code blocks
	markdown = multipleNewlinesRegex.ReplaceAllString(markdown, "\n\n")

	// Restore escaped characters
	for placeholder, original := range map[string]string{
		"\u0001": "*",
		"\u0002": "_",
		"\u0003": "`",
		"\u0004": "~",
		"\u0005": "[",
		"\u0006": "]",
		"\u0007": "(",
		"\u0008": ")",
		"\u000E": "#",
		"\u000F": ">",
		"\u0010": "!",
	} {
		markdown = strings.ReplaceAll(markdown, placeholder, original)
	}

	return markdown
}
