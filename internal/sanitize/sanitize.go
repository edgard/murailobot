package sanitize

import (
	"bytes"
	"html"
	"regexp"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

// Policy represents a sanitization policy for text content
type Policy struct {
	policy   *bluemonday.Policy
	markdown goldmark.Markdown
}

// NewTelegramPolicy creates a new Policy for stripping HTML and markdown
func NewTelegramPolicy() *Policy {
	return &Policy{
		policy:   bluemonday.StrictPolicy(),
		markdown: goldmark.New(),
	}
}

// SanitizeText strips HTML and markdown from the input text
func (p *Policy) SanitizeText(text string) string {
	if text == "" {
		return ""
	}

	// Convert markdown to HTML
	var buf bytes.Buffer
	if err := p.markdown.Convert([]byte(text), &buf); err != nil {
		return text
	}

	// Replace HTML line breaks with newlines before sanitizing
	htmlText := buf.String()
	htmlText = regexp.MustCompile(`<br\s*/?>|</?p>|</?div>|</?pre>|</?h[1-6]>`).ReplaceAllString(htmlText, "\n")

	// Sanitize HTML
	sanitized := p.policy.Sanitize(htmlText)

	// Normalize multiple newlines to a single one
	sanitized = regexp.MustCompile(`\n\s*\n+`).ReplaceAllString(sanitized, "\n\n")

	// Convert HTML entities back to characters
	sanitized = html.UnescapeString(sanitized)

	return sanitized
}
