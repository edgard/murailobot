package utils

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Threshold constants.
const (
	minNewlinesThreshold    = 3
	minHorizontalRuleLength = 3
	minMarkdownLinkGroups   = 3
	markdownHeaderMinLevel  = 1
	markdownHeaderMaxLevel  = 6
)

// Replacer groups.
var (
	// Unicode control/formatting character replacer.
	unicodeReplacer = strings.NewReplacer(
		"\u2060", "", "\u180E", "",
		"\u2028", "\n", "\u2029", "\n\n",
		"\u200B", " ", "\u200C", " ",
		"\u200D", "", "\uFEFF", "",
		"\u00AD", "", "\u205F", " ",
		"\u202A", "", "\u202B", "",
		"\u202C", "", "\u202D", "", "\u202E", "",
	)

	// Escaped markdown symbol replacer.
	escapedReplacer = strings.NewReplacer(
		"\\*", "\u0001", "\\_", "\u0002",
		"\\`", "\u0003", "\\~", "\u0004",
		"\\[", "\u0005", "\\]", "\u0006",
		"\\(", "\u0007", "\\)", "\u0008",
		"\\#", "\u000E", "\\>", "\u000F",
		"\\!", "\u0010",
	)

	// Original symbol restoration replacer.
	restoreReplacer = strings.NewReplacer(
		"\u0001", "*", "\u0002", "_",
		"\u0003", "`", "\u0004", "~",
		"\u0005", "[", "\u0006", "]",
		"\u0007", "(", "\u0008", ")",
		"\u000E", "#", "\u000F", ">", "\u0010", "!",
	)
)

// Regex pattern groups.
var (
	// Basic character patterns.
	controlCharsRegex     = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`)
	multipleNewlinesRegex = regexp.MustCompile("\n{" + strconv.Itoa(minNewlinesThreshold) + ",}")
	horizontalRuleRegex   = regexp.MustCompile("^[\\*\\-_]{" + strconv.Itoa(minHorizontalRuleLength) + ",}$")

	// Code block patterns.
	regexFencedCodeBlocks = regexp.MustCompile("```[\\s\\S]*?```")
	regexInlineCode       = regexp.MustCompile("`[^`]+`")

	// Link patterns.
	regexImages = regexp.MustCompile(`!\[(.*?)\]\(([^)]+)\)`)
	regexLinks  = regexp.MustCompile(`\[(.*?)\]\(([^)]+)\)`)

	// Text formatting patterns.
	regexHeaders = regexp.MustCompile("(?m)^#{" + strconv.Itoa(markdownHeaderMinLevel) + "," + strconv.Itoa(markdownHeaderMaxLevel) + "} (.+)$")
	regexBold    = regexp.MustCompile(`\*\*(.*?)\*\*`)
	regexBold2   = regexp.MustCompile(`__(.+?)__`)
	regexItalic  = regexp.MustCompile(`\*([^*]+)\*`)
	regexItalic2 = regexp.MustCompile(`_([^_]+)_`)
	regexStrike  = regexp.MustCompile(`~~(.+?)~~`)

	// List patterns.
	regexOrderedList  = regexp.MustCompile(`^\s*\d+\.\s+`)
	regexNumberedList = regexp.MustCompile(`^\d+\.\s+`)
	regexBlockquotes  = regexp.MustCompile(`(?m)^>\s*(.+)$`)

	// HTML and additional patterns.
	regexHTMLTags = regexp.MustCompile(`<[^>]*>`)

	// Markdown detection patterns.
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

// Regular functions.
func applyRegexReplacements(s string, rules []struct {
	re   *regexp.Regexp
	repl string
},
) string {
	// Execute each rule sequentially.
	return func() string {
		for _, rule := range rules {
			s = rule.re.ReplaceAllString(s, rule.repl)
		}

		return s
	}()
}

// String processing functions.
func normalizeLineWhitespace(line string) string {
	var b strings.Builder

	var space bool // space default false

	for _, r := range line {
		switch {
		case r == '\u3000':
			b.WriteRune(r)

			space = false
		case unicode.IsSpace(r) || r == '\u00A0':
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
				var s string
				s = strings.Trim(l, "|")
				s = strings.ReplaceAll(s, "|", " ")
				out = append(out, strings.TrimSpace(s))
				i++

				continue
			} else if strings.Contains(l, "---") && i > 0 && strings.HasPrefix(lines[i-1], "|") {
				continue
			}

			var s string
			s = strings.Trim(l, "|")
			s = strings.ReplaceAll(s, "|", " ")
			out = append(out, strings.TrimSpace(s))

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

// Public markdown functions.
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
		{regexHeaders, "$1"},
		{regexBold, "$1"},
		{regexBold2, "$1"},
		{regexItalic, "$1"},
		{regexItalic2, "$1"},
		{regexStrike, "$1"},
		{regexBlockquotes, "$1"},
	}

	md = applyRegexReplacements(md, rules)
	// Custom replacement for markdown links
	md = regexLinks.ReplaceAllStringFunc(md, func(match string) string {
		groups := regexLinks.FindStringSubmatch(match)
		if len(groups) < minMarkdownLinkGroups {
			return match
		}
		// If link text equals the URL, display just the URL.
		if groups[1] == groups[2] {
			return groups[2]
		}

		return groups[1] + " (" + groups[2] + ")"
	})

	md = processMarkdownStructures(md)
	md = regexHTMLTags.ReplaceAllString(md, "")
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
