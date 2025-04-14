// Package util provides utility functions for MurailoBot
package util

import (
	"fmt"
	"strings"
)

// Sanitize cleans up a text response, trimming whitespace and handling potential errors
func Sanitize(text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("empty response")
	}

	// Trim spaces, tabs, and newlines from beginning and end
	text = strings.TrimSpace(text)

	return text, nil
}

// EstimateTokens provides a rough estimate of token count for a text string
// This is a simple heuristic that works reasonably well for English text
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}

	// Split on whitespace
	words := strings.Fields(text)
	wordCount := len(words)

	// Count punctuation and other token separators
	separatorCount := 0
	for _, c := range text {
		switch c {
		case '.', ',', '!', '?', ':', ';', '(', ')', '[', ']', '{', '}', '"', '\'', '-', '+', '=', '/', '\\', '*', '&', '^', '%', '$', '#', '@', '<', '>', '|', '~', '`':
			separatorCount++
		}
	}

	// Very simple token estimate: each word is roughly 1.3 tokens,
	// plus a small addition for separators
	return int(float64(wordCount)*1.3) + (separatorCount / 2)
}
