package utils

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Threshold constants define limits for various text processing operations.
const (
	minNewlinesThreshold    = 3 // Minimum consecutive newlines to collapse
	minHorizontalRuleLength = 3 // Minimum length for horizontal rule detection
	minMarkdownLinkGroups   = 3 // Expected capture groups in markdown link regex
	markdownHeaderMinLevel  = 1 // Minimum header level (#)
	markdownHeaderMaxLevel  = 6 // Maximum header level (######)
)

// Replacer groups define character substitution mappings for text processing.
var (
	// unicodeReplacer handles special Unicode control and formatting characters,
	// converting them to their appropriate plain text equivalents.
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

	// markdownRegexps contains all patterns used to detect markdown formatting.
	// This comprehensive list helps identify if text contains markdown syntax.
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

// applyRegexReplacements applies a series of regular expression replacements
// to a string in sequence. Each replacement rule consists of a regular expression
// and its replacement pattern.
func applyRegexReplacements(s string, rules []struct {
	re   *regexp.Regexp
	repl string
},
) string {
	for _, rule := range rules {
		s = rule.re.ReplaceAllString(s, rule.repl)
	}

	return s
}

// normalizeLineWhitespace normalizes whitespace within a single line of text:
// - Collapses multiple spaces into a single space
// - Preserves ideographic spaces (U+3000)
// - Converts other Unicode whitespace to regular spaces
// - Trims leading and trailing whitespace.
func normalizeLineWhitespace(line string) string {
	var b strings.Builder

	var space bool // space default false

	for _, r := range line {
		switch {
		case r == '\u3000': // Preserve ideographic space
			b.WriteRune(r)

			space = false
		case unicode.IsSpace(r) || r == '\u00A0': // Handle all other whitespace
			if !space {
				b.WriteRune(' ')

				space = true
			}
		default:
			b.WriteRune(r)

			space = false
		}
	}

	return strings.TrimSpace(b.String())
}

// processListLine removes markdown list markers from a line while preserving indentation.
func processListLine(l string) string {
	trim := strings.TrimSpace(l)
	if strings.HasPrefix(trim, "* ") || strings.HasPrefix(trim, "- ") || strings.HasPrefix(trim, "+ ") {
		indent := l[:len(l)-len(trim)]

		return indent + strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(trim, "* "), "- "), "+ "))
	} else if orderedListRegex.MatchString(l) {
		trim = strings.TrimSpace(l)
		indent := l[:len(l)-len(trim)]

		return indent + numberedListRegex.ReplaceAllString(trim, "")
	}

	return l
}

// processTableRowAndRule handles markdown table rows and separator lines.
func processTableRowAndRule(lines []string, i int) (int, string, bool) {
	l := lines[i]
	if strings.HasPrefix(l, "|") && strings.HasSuffix(l, "|") {
		if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "|") &&
			strings.HasSuffix(lines[i+1], "|") && strings.Contains(lines[i+1], "---") {
			s := strings.Trim(l, "|")
			s = strings.ReplaceAll(s, "|", " ")
			i++

			return i, strings.TrimSpace(s), true
		} else if strings.Contains(l, "---") && i > 0 && strings.HasPrefix(lines[i-1], "|") {
			return i, "", true
		}

		s := strings.Trim(l, "|")
		s = strings.ReplaceAll(s, "|", " ")

		return i, strings.TrimSpace(s), true
	}

	return i, "", false
}

// processMarkdownStructures handles structural markdown elements including lists,
// tables, and horizontal rules.
func processMarkdownStructures(md string) string {
	lines := strings.Split(md, "\n")

	var out []string

	for i := 0; i < len(lines); {
		newIndex, processed, handled := processTableRowAndRule(lines, i)
		if handled {
			if processed != "" {
				out = append(out, processed)
			}

			i = newIndex + 1

			continue
		}

		l := processListLine(lines[i])

		if horizontalRuleRegex.MatchString(strings.TrimSpace(l)) {
			i++

			continue
		}

		out = append(out, l)
		i++
	}

	return strings.Join(out, "\n")
}

// IsMarkdown determines if a text string contains markdown formatting
// by checking against a comprehensive set of markdown patterns.
// It handles special cases like escaped asterisks to avoid false positives.
func IsMarkdown(text string) bool {
	for _, re := range markdownRegexps {
		if re.MatchString(text) {
			// Special case: differentiate escaped asterisks
			if strings.Contains(re.String(), `\*`) && strings.Contains(text, `\*`) {
				esc := regexp.MustCompile(`\\[\*]`)
				unesc := regexp.MustCompile(`[^\\]\*`)

				if esc.FindAllString(text, -1) != nil && unesc.FindAllString(text, -1) == nil {
					continue
				}
			}

			return true
		}
	}

	return false
}

// stripMarkdown removes markdown formatting from text while preserving the content.
// It handles:
// - Code blocks and inline code
// - Images and links
// - Headers and text formatting (bold, italic, strikethrough)
// - Lists and blockquotes
// - Tables and horizontal rules
// - HTML tags
//
// The function preserves escaped markdown symbols and maintains
// reasonable whitespace formatting.
func stripMarkdown(md string) string {
	md = escapedReplacer.Replace(md)
	rules := []struct {
		re   *regexp.Regexp
		repl string
	}{
		{fencedCodeBlocksRegex, "\n"},
		{inlineCodeRegex, ""},
		{imagesRegex, "$2"},
		{headersRegex, "$1"},
		{boldRegex, "$1"},
		{boldAltRegex, "$1"},
		{italicRegex, "$1"},
		{italicAltRegex, "$1"},
		{strikeRegex, "$1"},
		{blockquotesRegex, "$1"},
	}

	md = applyRegexReplacements(md, rules)
	// Custom replacement for markdown links
	md = linksRegex.ReplaceAllStringFunc(md, func(match string) string {
		groups := linksRegex.FindStringSubmatch(match)
		if len(groups) < minMarkdownLinkGroups {
			return match
		}
		// If link text equals the URL, display just the URL
		if groups[1] == groups[2] {
			return groups[2]
		}

		return groups[1] + " (" + groups[2] + ")"
	})

	md = processMarkdownStructures(md)
	md = htmlTagsRegex.ReplaceAllString(md, "")
	md = multipleNewlinesRegex.ReplaceAllString(md, "\n\n")

	return restoreReplacer.Replace(md)
}

// Sanitize normalizes text by:
// - Removing control characters
// - Normalizing whitespace
// - Converting markdown to plain text
// - Normalizing newlines
// - Removing excessive blank lines
//
// The function is safe to use with any text input and preserves
// meaningful formatting while removing potentially problematic characters.
func Sanitize(input string) string {
	if input == "" {
		return ""
	}

	if IsMarkdown(input) {
		input = stripMarkdown(input)
	}

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

	return strings.TrimSpace(s)
}
