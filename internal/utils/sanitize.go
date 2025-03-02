package utils

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	// Group: Unicode replacers - Replaces Unicode control and formatting characters that affect markdown rendering.
	unicodeReplacer = strings.NewReplacer(
		"\u2060", "", "\u180E", "",
		"\u2028", "\n", "\u2029", "\n\n",
		"\u200B", " ", "\u200C", " ",
		"\u200D", "", "\uFEFF", "",
		"\u00AD", "", "\u205F", " ",
		"\u202A", "", "\u202B", "",
		"\u202C", "", "\u202D", "", "\u202E", "",
	)

	// Group: Escaped character replacers - Temporarily substitutes escaped markdown symbols with placeholder characters.
	escapedReplacer = strings.NewReplacer(
		"\\*", "\u0001", "\\_", "\u0002",
		"\\`", "\u0003", "\\~", "\u0004",
		"\\[", "\u0005", "\\]", "\u0006",
		"\\(", "\u0007", "\\)", "\u0008",
		"\\#", "\u000E", "\\>", "\u000F",
		"\\!", "\u0010",
	)

	// Group: Restoration replacers - Converts placeholder characters back to their original markdown symbols.
	restoreReplacer = strings.NewReplacer(
		"\u0001", "*", "\u0002", "_",
		"\u0003", "`", "\u0004", "~",
		"\u0005", "[", "\u0006", "]",
		"\u0007", "(", "\u0008", ")",
		"\u000E", "#", "\u000F", ">", "\u0010", "!",
	)
)

var (
	controlCharsRegex     = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`) // Matches control characters (non-printable ranges).
	multipleNewlinesRegex = regexp.MustCompile("\n{3,}")                           // Matches sequences of 3 or more newlines.
	horizontalRuleRegex   = regexp.MustCompile(`^[\*\-_]{3,}$`)                    // Matches a horizontal rule made entirely of *, -, or _.

	regexFencedCodeBlocks = regexp.MustCompile("```[\\s\\S]*?```") // Matches fenced code blocks.
	regexInlineCode       = regexp.MustCompile("`[^`]+`")          // Matches inline code delimited by backticks.

	regexImages = regexp.MustCompile(`!\[(.*?)\]\(([^)]+)\)`) // Matches markdown image syntax.
	regexLinks  = regexp.MustCompile(`\[(.*?)\]\(([^)]+)\)`)  // Matches markdown link syntax.

	regexHeaders = regexp.MustCompile(`(?m)^#{1,6} (.+)$`) // Matches markdown headers (lines starting with 1-6 '#' and a space).
	regexBold    = regexp.MustCompile(`\*\*(.*?)\*\*`)     // Matches bold text enclosed in **.
	regexBold2   = regexp.MustCompile(`__(.+?)__`)         // Matches bold text enclosed in __.
	regexItalic  = regexp.MustCompile(`\*([^*]+)\*`)       // Matches italic text enclosed in *.
	regexItalic2 = regexp.MustCompile(`_([^_]+)_`)         // Matches italic text enclosed in _.
	regexStrike  = regexp.MustCompile(`~~(.+?)~~`)         // Matches strikethrough text enclosed in ~~.

	regexOrderedList  = regexp.MustCompile(`^\s*\d+\.\s+`)   // Matches unordered list markers (optional whitespace, number, dot, and space).
	regexNumberedList = regexp.MustCompile(`^\d+\.\s+`)      // Matches numbered list markers at the beginning of a line.
	regexBlockquotes  = regexp.MustCompile(`(?m)^>\s*(.+)$`) // Matches blockquote lines (starting with '>').

	regexHtmlTags = regexp.MustCompile(`<[^>]*>`) // Matches any HTML tag.

	markdownRegexes = []*regexp.Regexp{
		regexp.MustCompile(`\*\*.+?\*\*`),                 // Bold with **.
		regexp.MustCompile(`__.+?__`),                     // Bold with __.
		regexp.MustCompile(`\*.+?\*`),                     // Italics with *.
		regexp.MustCompile(`_.+?_`),                       // Italics with _.
		regexp.MustCompile(`~~.+?~~`),                     // Strikethrough.
		regexp.MustCompile(`\[.+?\]\(.+?\)`),              // Markdown links.
		regexp.MustCompile(`!\[.+?\]\(.+?\)`),             // Markdown images.
		regexp.MustCompile("```[\\s\\S]+?```"),            // Fenced code blocks.
		regexp.MustCompile("`[^`]+`"),                     // Inline code.
		regexp.MustCompile(`(?m)^#{1,6} .+$`),             // Markdown headers detection.
		regexp.MustCompile(`(?m)^> .+$`),                  // Blockquotes detection.
		regexp.MustCompile(`(?m)^[\*\-\+] .+$`),           // Unordered list detection.
		regexp.MustCompile(`(?m)^\d+\. .+$`),              // Ordered list detection.
		regexp.MustCompile(`(?m)^[\*\-_]{3,}$`),           // Horizontal rule detection.
		regexp.MustCompile(`(?m)^\|.+\|$`),                // Table row detection.
		regexp.MustCompile(`(?m)^.+\r?\n(=+|-+)\s*$`),     // Setext-style headers detection.
		regexp.MustCompile(`(?m)^[-*] \[(?: |x|X)\] .+$`), // Task list detection.
		regexp.MustCompile(`(?m)^\[\^.+\]:\s+.+$`),        // Footnote detection.
		regexp.MustCompile(`(?m)^( {4}|\t).+`),            // Indented code block detection.
		regexp.MustCompile(`(?m)\$[^$\n]+\$`),             // Inline math detection.
		regexp.MustCompile(`(?m)\$\$[\s\S]+\$\$`),         // Display math detection.
	}
)

// applyRegexReplacements applies a series of regex replacement rules to s.
func applyRegexReplacements(s string, rules []struct {
	re   *regexp.Regexp
	repl string
}) string {
	// Execute each rule sequentially.
	return func() string {
		for _, rule := range rules {
			s = rule.re.ReplaceAllString(s, rule.repl)
		}
		return s
	}()
}

// normalizeLineWhitespace collapses multiple whitespace characters in a line,
// preserving fullwidth spaces.
func normalizeLineWhitespace(line string) string {
	var b strings.Builder
	space := false

	for _, r := range line {
		if r == '\u3000' {
			b.WriteRune(r)
			space = false
		} else if unicode.IsSpace(r) || r == '\u00A0' {
			if !space {
				b.WriteRune(' ')
				space = true
			}
		} else {
			b.WriteRune(r)
			space = false
		}
	}
	return strings.TrimSpace(b.String())
}

// processMarkdownStructures normalizes Markdown list markers, tables, and horizontal rules.
func processMarkdownStructures(md string) string {
	lines := strings.Split(md, "\n")
	var out []string

	for i := 0; i < len(lines); i++ {
		l := lines[i]
		trim := strings.TrimSpace(l)

		// Remove marker characters for unordered lists.
		if strings.HasPrefix(trim, "* ") || strings.HasPrefix(trim, "- ") || strings.HasPrefix(trim, "+ ") {
			indent := l[:len(l)-len(trim)]
			l = indent + strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(trim, "* "), "- "), "+ "))
		} else if regexOrderedList.MatchString(l) {
			// Normalize ordered list markers.
			trim = strings.TrimSpace(l)
			indent := l[:len(l)-len(trim)]
			l = indent + regexNumberedList.ReplaceAllString(trim, "")
		}

		// Normalize table rows.
		if strings.HasPrefix(l, "|") && strings.HasSuffix(l, "|") {
			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "|") &&
				strings.HasSuffix(lines[i+1], "|") && strings.Contains(lines[i+1], "---") {
				out = append(out, strings.TrimSpace(strings.ReplaceAll(strings.Trim(l, "|"), "|", " ")))
				i++
				continue
			} else if strings.Contains(l, "---") && i > 0 && strings.HasPrefix(lines[i-1], "|") {
				continue
			}
			out = append(out, strings.TrimSpace(strings.ReplaceAll(strings.Trim(l, "|"), "|", " ")))
			continue
		}

		// Skip horizontal rules.
		if horizontalRuleRegex.MatchString(strings.TrimSpace(l)) {
			continue
		}

		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// IsMarkdown checks if the given text contains Markdown formatting.
func IsMarkdown(text string) bool {
	for _, re := range markdownRegexes {
		if re.MatchString(text) {
			// Special case: differentiate escaped asterisks.
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

// StripMarkdown removes Markdown formatting and returns plain text.
func StripMarkdown(md string) string {
	md = escapedReplacer.Replace(md)
	rules := []struct {
		re   *regexp.Regexp
		repl string
	}{
		{regexFencedCodeBlocks, "\n"},
		{regexInlineCode, ""},
		{regexImages, "$2"},
		{regexLinks, "$1 ($2)"},
		{regexHeaders, "$1"},
		{regexBold, "$1"},
		{regexBold2, "$1"},
		{regexItalic, "$1"},
		{regexItalic2, "$1"},
		{regexStrike, "$1"},
		{regexBlockquotes, "$1"},
	}

	md = applyRegexReplacements(md, rules)
	md = processMarkdownStructures(md)
	md = regexHtmlTags.ReplaceAllString(md, "")
	md = multipleNewlinesRegex.ReplaceAllString(md, "\n\n")
	return restoreReplacer.Replace(md)
}

// Sanitize normalizes a string by removing control characters,
// normalizing whitespace, and converting Markdown to plain text.
func Sanitize(input string) string {
	if input == "" {
		return ""
	}

	if IsMarkdown(input) {
		input = StripMarkdown(input)
	}

	s := strings.ReplaceAll(input, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = unicodeReplacer.Replace(s)
	s = controlCharsRegex.ReplaceAllString(s, " ")

	parts := strings.Split(s, "\n")
	for i, p := range parts {
		parts[i] = normalizeLineWhitespace(p)
	}
	s = strings.Join(parts, "\n")
	s = multipleNewlinesRegex.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
