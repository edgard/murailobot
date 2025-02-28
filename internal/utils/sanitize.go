package utils

import (
	"bytes"
	"html"
	"regexp"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

// Sanitize cleans and formats text by converting markdown to HTML
// and removing potentially unsafe content
func Sanitize(text string) string {
	if text == "" {
		return ""
	}

	policy := bluemonday.StrictPolicy()
	markdown := goldmark.New()

	var buf bytes.Buffer
	if err := markdown.Convert([]byte(text), &buf); err != nil {
		return text
	}

	htmlText := buf.String()

	htmlText = regexp.MustCompile(
		`<br\s*/?>|</?p>|</?div>|</?pre>|</?h[1-6]>`,
	).ReplaceAllString(htmlText, "\n")

	sanitized := policy.Sanitize(htmlText)
	sanitized = regexp.MustCompile(`\n\s*\n+`).ReplaceAllString(sanitized, "\n\n")
	sanitized = html.UnescapeString(sanitized)
	sanitized = regexp.MustCompile(`\s{2,}`).ReplaceAllString(sanitized, " ")

	return sanitized
}
