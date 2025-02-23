// Package utils provides text sanitization for messaging platforms.
package utils

import (
	"bytes"
	"html"
	"regexp"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

const sanitizeComponent = "sanitize"

// TextPolicy combines HTML sanitization and Markdown processing
type TextPolicy struct {
	policy   *bluemonday.Policy
	markdown goldmark.Markdown
}

// NewTelegramTextPolicy creates strict policy for Telegram
func NewTelegramTextPolicy() *TextPolicy {
	return &TextPolicy{
		policy:   bluemonday.StrictPolicy(),
		markdown: goldmark.New(),
	}
}

// SanitizeText processes text through:
// 1. Markdown to HTML conversion
// 2. Block elements to newlines
// 3. HTML tag removal
// 4. Whitespace normalization
// 5. Entity decoding
func (p *TextPolicy) SanitizeText(text string) string {
	if text == "" {
		return ""
	}

	var buf bytes.Buffer
	if err := p.markdown.Convert([]byte(text), &buf); err != nil {
		WriteWarnLog(sanitizeComponent, "failed to convert markdown",
			KeyError, err.Error(),
			KeySize, len(text),
			KeyAction, "markdown_convert",
			KeyType, "sanitize")
		return text
	}

	htmlText := buf.String()
	// Replace block elements with newlines:
	// <br>, <p>, <div>, <pre>, <h1-6>
	htmlText = regexp.MustCompile(
		`<br\s*/?>|</?p>|</?div>|</?pre>|</?h[1-6]>`,
	).ReplaceAllString(htmlText, "\n")

	sanitized := p.policy.Sanitize(htmlText)
	sanitized = regexp.MustCompile(`\n\s*\n+`).ReplaceAllString(sanitized, "\n\n")
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
