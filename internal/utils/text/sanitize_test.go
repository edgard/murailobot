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
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "plain text",
			input:    "Hello, world!",
			expected: "Hello, world!",
		},
		{
			name:     "markdown formatting",
			input:    "**Bold** and *italic* text",
			expected: "Bold and italic text",
		},
		{
			name:     "markdown list",
			input:    "* Item 1\n* Item 2",
			expected: "Item 1\nItem 2",
		},
		{
			name:     "markdown table",
			input:    "| Header 1 | Header 2 |\n|----------|----------|\n| Cell 1   | Cell 2   |",
			expected: "Header 1 Header 2\nCell 1 Cell 2",
		},
		{
			name:     "control characters",
			input:    "Text with\x00control\x1Fcharacters",
			expected: "Text with control characters",
		},
		{
			name:     "multiple newlines",
			input:    "Line 1\n\n\n\nLine 2",
			expected: "Line 1\n\nLine 2",
		},
		{
			name:     "unicode whitespace",
			input:    "Text with\u2028line\u2029breaks",
			expected: "Text with\nline\n\nbreaks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := text.Sanitize(tt.input); got != tt.expected {
				t.Errorf("Sanitize() = %q, want %q", got, tt.expected)
			}
		})
	}
}
