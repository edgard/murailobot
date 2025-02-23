package utils

import (
	"bytes"
	"html"
	"regexp"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

const sanitizeComponent = "sanitize"

// TextPolicy represents a sanitization policy for text content
type TextPolicy struct {
	policy   *bluemonday.Policy
	markdown goldmark.Markdown
}

// NewTelegramTextPolicy creates a new Policy for stripping HTML and markdown
// suitable for Telegram messages
func NewTelegramTextPolicy() *TextPolicy {
	return &TextPolicy{
		policy:   bluemonday.StrictPolicy(),
		markdown: goldmark.New(),
	}
}

// SanitizeText strips HTML and markdown from the input text
func (p *TextPolicy) SanitizeText(text string) string {
	if text == "" {
		return ""
	}

	// Convert markdown to HTML
	var buf bytes.Buffer
	if err := p.markdown.Convert([]byte(text), &buf); err != nil {
		WriteWarnLog(sanitizeComponent, "failed to convert markdown",
			KeyError, err.Error(),
			KeySize, len(text),
			KeyAction, "markdown_convert",
			KeyType, "sanitize")
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

	WriteDebugLog(sanitizeComponent, "text sanitized successfully",
		KeyAction, "sanitize_text",
		KeyType, "sanitize",
		KeySize, map[string]int{
			"input":    len(text),
			"output":   len(sanitized),
			"markdown": len(htmlText),
		})

	return sanitized
}
