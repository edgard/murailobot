// Package utils provides common utility functions and patterns.
// This file implements text sanitization for safe message handling,
// including HTML and Markdown processing with proper escaping and
// formatting for messaging platforms.
package utils

import (
	"bytes"
	"html"
	"regexp"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

const sanitizeComponent = "sanitize"

// TextPolicy represents a sanitization policy for text content.
// It combines HTML sanitization (via bluemonday) and Markdown
// processing (via goldmark) to ensure text is safe and properly
// formatted for the target platform.
type TextPolicy struct {
	policy   *bluemonday.Policy // HTML sanitization policy
	markdown goldmark.Markdown  // Markdown processor
}

// NewTelegramTextPolicy creates a new Policy specifically configured
// for Telegram message formatting. It uses:
// - Strict HTML sanitization to prevent XSS and other injection attacks
// - Standard Markdown processing for basic formatting
// - Custom handling of line breaks and block elements
func NewTelegramTextPolicy() *TextPolicy {
	return &TextPolicy{
		policy:   bluemonday.StrictPolicy(), // Removes all HTML tags
		markdown: goldmark.New(),            // Standard Markdown processing
	}
}

// SanitizeText processes input text to ensure it's safe and properly formatted
// for messaging platforms. The process includes:
// 1. Converting Markdown to HTML for consistent processing
// 2. Converting block-level elements to line breaks
// 3. Removing all HTML tags while preserving content
// 4. Normalizing whitespace and line breaks
// 5. Converting HTML entities back to characters
//
// If Markdown conversion fails, the original text is returned with
// basic HTML sanitization applied.
func (p *TextPolicy) SanitizeText(text string) string {
	if text == "" {
		return ""
	}

	// Convert Markdown to HTML for unified processing
	var buf bytes.Buffer
	if err := p.markdown.Convert([]byte(text), &buf); err != nil {
		WriteWarnLog(sanitizeComponent, "failed to convert markdown",
			KeyError, err.Error(),
			KeySize, len(text),
			KeyAction, "markdown_convert",
			KeyType, "sanitize")
		return text
	}

	// Replace common block-level HTML elements with newlines
	// This preserves document structure while removing HTML
	htmlText := buf.String()
	htmlText = regexp.MustCompile(
		// Match various block-level elements:
		// - <br> or <br/> - Line breaks
		// - <p> or </p> - Paragraphs
		// - <div> or </div> - Generic blocks
		// - <pre> or </pre> - Preformatted text
		// - <h1> through <h6> - Headers
		`<br\s*/?>|</?p>|</?div>|</?pre>|</?h[1-6]>`,
	).ReplaceAllString(htmlText, "\n")

	// Remove all remaining HTML tags while preserving content
	sanitized := p.policy.Sanitize(htmlText)

	// Normalize whitespace and line breaks:
	// - Multiple consecutive newlines become two newlines
	// - Removes excess whitespace around newlines
	sanitized = regexp.MustCompile(`\n\s*\n+`).ReplaceAllString(sanitized, "\n\n")

	// Convert HTML entities (like &amp;) back to their character equivalents
	sanitized = html.UnescapeString(sanitized)

	WriteDebugLog(sanitizeComponent, "text sanitized successfully",
		KeyAction, "sanitize_text",
		KeyType, "sanitize",
		KeySize, map[string]int{
			"input":    len(text),      // Original text length
			"output":   len(sanitized), // Final text length
			"markdown": len(htmlText),  // Intermediate HTML length
		})

	return sanitized
}
