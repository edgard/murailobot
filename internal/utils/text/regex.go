// Package text provides text sanitization and markdown processing.
package text

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	unicodeReplacer = strings.NewReplacer(
		"\u2060", "", "\u180E", "",
		"\u2028", "\n", "\u2029", "\n\n",
		"\u200B", " ", "\u200C", " ",
		"\u200D", "", "\uFEFF", "",
		"\u00AD", "", "\u205F", " ",
		"\u202A", "", "\u202B", "",
		"\u202C", "", "\u202D", "", "\u202E", "",
	)

	escapedReplacer = strings.NewReplacer(
		"\\*", "\u0001", "\\_", "\u0002",
		"\\`", "\u0003", "\\~", "\u0004",
		"\\[", "\u0005", "\\]", "\u0006",
		"\\(", "\u0007", "\\)", "\u0008",
		"\\#", "\u000E", "\\>", "\u000F",
		"\\!", "\u0010",
	)

	restoreReplacer = strings.NewReplacer(
		"\u0001", "*", "\u0002", "_",
		"\u0003", "`", "\u0004", "~",
		"\u0005", "[", "\u0006", "]",
		"\u0007", "(", "\u0008", ")",
		"\u000E", "#", "\u000F", ">", "\u0010", "!",
	)
)

var (
	controlCharsRegex     = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`)
	multipleNewlinesRegex = regexp.MustCompile("\n{" + strconv.Itoa(minNewlinesThreshold) + ",}")
	horizontalRuleRegex   = regexp.MustCompile("^[\\*\\-_]{" + strconv.Itoa(minHorizontalRuleLength) + ",}$")
	setextHeaderRegex     = regexp.MustCompile(`(?m)^[^\n]+\n[=\-]{2,}$`)
	fencedCodeBlocksRegex = regexp.MustCompile("```[\\s\\S]*?```")
	inlineCodeRegex       = regexp.MustCompile("`[^`]+`")
	imagesRegex           = regexp.MustCompile(`!\[(.*?)\]\(([^)]+)\)`)
	linksRegex            = regexp.MustCompile(`\[(.*?)\]\(([^)]+)\)`)
	headersRegex          = regexp.MustCompile("(?m)^#{" + strconv.Itoa(markdownHeaderMinLevel) + "," + strconv.Itoa(markdownHeaderMaxLevel) + "} (.+)$")
	boldRegex             = regexp.MustCompile(`\*\*(.*?)\*\*`)
	boldAltRegex          = regexp.MustCompile(`__(.+?)__`)
	italicRegex           = regexp.MustCompile(`\*([^*]+)\*`)
	italicAltRegex        = regexp.MustCompile(`_([^_]+)_`)
	strikeRegex           = regexp.MustCompile(`~~(.+?)~~`)
	orderedListRegex      = regexp.MustCompile(`^\s*\d+\.\s+`)
	numberedListRegex     = regexp.MustCompile(`^\d+\.\s+`)
	blockquotesRegex      = regexp.MustCompile(`(?m)^>\s*(.+)$`)
	htmlTagsRegex         = regexp.MustCompile(`<[^>]*>`)
	metadataFormatRegex   = regexp.MustCompile(`^\s*\[\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})\]\s+[^:]+:\s*`)
)

var (
	unorderedListRegex = regexp.MustCompile(`(?m)^[\*\-\+] .+$`)
	tableRowRegex      = regexp.MustCompile(`(?m)^\|.+\|$`)
)

// markdownRegexps contains patterns for detecting markdown syntax.
var markdownRegexps = []*regexp.Regexp{
	boldRegex,
	boldAltRegex,
	italicRegex,
	italicAltRegex,
	strikeRegex,
	linksRegex,
	imagesRegex,
	fencedCodeBlocksRegex,
	inlineCodeRegex,
	headersRegex,
	setextHeaderRegex,
	blockquotesRegex,
	unorderedListRegex,
	orderedListRegex,
	horizontalRuleRegex,
	tableRowRegex,
}
