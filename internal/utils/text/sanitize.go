// Package text provides text sanitization and markdown processing.
package text

import (
	"regexp"
	"strings"
	"unicode"
)

func applyRegexReplacements(s string, rules []struct {
	regex       *regexp.Regexp
	replacement string
},
) string {
	for _, rule := range rules {
		s = rule.regex.ReplaceAllString(s, rule.replacement)
	}

	return s
}

func normalizeLineWhitespace(line string) string {
	var strBuilder strings.Builder

	var space bool

	for _, r := range line {
		switch {
		case r == '\u3000':
			strBuilder.WriteRune(r)

			space = false
		case unicode.IsSpace(r) || r == '\u00A0':
			if !space {
				strBuilder.WriteRune(' ')

				space = true
			}
		default:
			strBuilder.WriteRune(r)

			space = false
		}
	}

	return strings.TrimSpace(strBuilder.String())
}

func processListLine(l string) string {
	trim := strings.TrimSpace(l)
	indent := l[:len(l)-len(trim)]

	if unorderedListRegex.MatchString(l) {
		// Remove the list marker (* or - or +) and its trailing space
		for _, prefix := range []string{"* ", "- ", "+ "} {
			if strings.HasPrefix(trim, prefix) {
				return indent + strings.TrimSpace(strings.TrimPrefix(trim, prefix))
			}
		}
	} else if orderedListRegex.MatchString(l) {
		// Remove the number and dot prefix
		return indent + numberedListRegex.ReplaceAllString(trim, "")
	}

	return l
}

func processTableRowAndRule(lines []string, i int) (int, string, bool) {
	l := lines[i]
	if strings.HasPrefix(l, "|") && strings.HasSuffix(l, "|") {
		if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "|") &&
			strings.HasSuffix(lines[i+1], "|") && strings.Contains(lines[i+1], "---") {
			rowContent := strings.Trim(l, "|")
			rowContent = strings.ReplaceAll(rowContent, "|", " ")
			i++

			return i, strings.TrimSpace(rowContent), true
		} else if strings.Contains(l, "---") && i > 0 && strings.HasPrefix(lines[i-1], "|") {
			return i, "", true
		}

		rowContent := strings.Trim(l, "|")
		rowContent = strings.ReplaceAll(rowContent, "|", " ")

		return i, strings.TrimSpace(rowContent), true
	}

	return i, "", false
}

func processMarkdownStructures(md string) string {
	lines := strings.Split(md, "\n")

	var processedLines []string

	for i := 0; i < len(lines); {
		newIndex, processed, handled := processTableRowAndRule(lines, i)
		if handled {
			if processed != "" {
				processedLines = append(processedLines, processed)
			}

			i = newIndex + 1

			continue
		}

		l := processListLine(lines[i])

		// Skip horizontal rules and setext header underlines
		trimmed := strings.TrimSpace(l)
		if horizontalRuleRegex.MatchString(trimmed) {
			// If this is part of a setext header, skip it but keep the header text
			i++

			continue
		} else if i > 0 && horizontalRuleRegex.MatchString(strings.TrimSpace(lines[i])) {
			// If next line is a horizontal rule, keep current line
			processedLines = append(processedLines, l)
			i += 2

			continue
		}

		processedLines = append(processedLines, l)
		i++
	}

	return strings.Join(processedLines, "\n")
}

func IsMarkdown(text string) bool {
	for _, re := range markdownRegexps {
		if re.MatchString(text) {
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

func stripMarkdown(md string) string {
	md = escapedReplacer.Replace(md)
	rules := []struct {
		regex       *regexp.Regexp
		replacement string
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
	md = linksRegex.ReplaceAllStringFunc(md, func(match string) string {
		groups := linksRegex.FindStringSubmatch(match)
		if len(groups) < minMarkdownLinkGroups {
			return match
		}

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

// Sanitize normalizes text by removing control characters and converting markdown to plain text.
func Sanitize(input string) string {
	if input == "" {
		return ""
	}

	input = metadataFormatRegex.ReplaceAllString(input, "")

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
