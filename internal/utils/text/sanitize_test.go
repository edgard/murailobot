package text_test

import (
	"testing"

	"github.com/edgard/murailobot/internal/utils/text"
)

func TestSanitize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "With metadata at beginning",
			input:    "[2025-03-06T22:30:11+01:00] UID 123456: This is a response with metadata.",
			expected: "This is a response with metadata.",
		},
		{
			name:     "With metadata and multiple lines",
			input:    "[2025-03-06T22:30:11+01:00] UID 123456: First line.\nSecond line.\nThird line.",
			expected: "First line.\nSecond line.\nThird line.",
		},
		{
			name:     "With metadata in middle of text (should not remove)",
			input:    "This text has [2025-03-06T22:30:11+01:00] UID 123456: in the middle.",
			expected: "This text has [2025-03-06T22:30:11+01:00] UID 123456: in the middle.",
		},
		{
			name:     "With metadata with fractional seconds",
			input:    "[2025-03-06T22:30:11.123+01:00] UID 123456: Response with fractional seconds.",
			expected: "Response with fractional seconds.",
		},
		{
			name:     "Without metadata",
			input:    "Just a regular message without metadata.",
			expected: "Just a regular message without metadata.",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Only metadata",
			input:    "[2025-03-06T22:30:11+01:00] UID 123456:",
			expected: "",
		},
		{
			name:     "With whitespace after metadata",
			input:    "[2025-03-06T22:30:11+01:00] UID 123456:    Message with leading spaces.",
			expected: "Message with leading spaces.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := text.Sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("Sanitize() = %v, want %v", result, tt.expected)
			}
		})
	}
}
