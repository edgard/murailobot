package common_test

import (
	"strings"
	"testing"

	"github.com/edgard/murailobot/internal/common"
)

func TestNormalizeText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic cases
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only spaces",
			input:    "   ",
			expected: "",
		},
		{
			name:     "only tabs",
			input:    "\t\t\t",
			expected: "",
		},
		{
			name:     "only newlines",
			input:    "\n\n\n\n",
			expected: "",
		},

		// Mixed whitespace
		{
			name:     "mixed whitespace characters",
			input:    " \t \n \t\n\t ",
			expected: "",
		},
		{
			name:     "non-breaking spaces",
			input:    "hello\u00A0\u00A0world", // actual non-breaking spaces
			expected: "hello world",
		},
		{
			name:     "en space",
			input:    "hello\u2002world", // en space
			expected: "hello world",
		},
		{
			name:     "em space",
			input:    "hello\u2003world", // em space
			expected: "hello world",
		},
		{
			name:     "thin space",
			input:    "hello\u2009world", // thin space
			expected: "hello world",
		},
		{
			name:     "ideographic space",
			input:    "hello\u3000world", // ideographic space
			expected: "hello world",
		},

		// Single line cases
		{
			name:     "single line with spaces",
			input:    "  hello   world  ",
			expected: "hello world",
		},
		{
			name:     "single line with tabs",
			input:    "\thello\t\tworld\t",
			expected: "hello world",
		},
		{
			name:     "multiple words with mixed spacing",
			input:    "word1\t \tword2    word3\t\t\tword4",
			expected: "word1 word2 word3 word4",
		},

		// Multiple line cases
		{
			name:     "two lines with mixed whitespace",
			input:    "  line1  \t  \n\t\tline2\t\t  ",
			expected: "line1\nline2",
		},
		{
			name:     "preserve single empty line",
			input:    "hello\n\nworld",
			expected: "hello\n\nworld",
		},
		{
			name:     "limit multiple empty lines",
			input:    "hello\n\n\n\n\nworld",
			expected: "hello\n\nworld",
		},
		{
			name:     "mixed content with empty lines",
			input:    "Title\n\nParagraph 1\n\n\n\nParagraph 2\n\n\n\n\n\nParagraph 3",
			expected: "Title\n\nParagraph 1\n\nParagraph 2\n\nParagraph 3",
		},
		{
			name:     "text with trailing newlines",
			input:    "Hello world\n\n\n\n",
			expected: "Hello world",
		},
		{
			name:     "text with leading newlines",
			input:    "\n\n\n\nHello world",
			expected: "Hello world",
		},

		// Newline variants
		{
			name:     "Windows line endings (CRLF)",
			input:    "line1\r\nline2\r\nline3",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "Mac classic line endings (CR)",
			input:    "line1\rline2\rline3",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "mixed line endings",
			input:    "line1\r\nline2\rline3\nline4",
			expected: "line1\nline2\nline3\nline4",
		},
		{
			name:     "complex mixed line endings",
			input:    "line1\r\n\r\nline2\r\r\nline3\n\r\n\rline4",
			expected: "line1\n\nline2\n\nline3\n\nline4",
		},

		// Whitespace-only lines
		{
			name:     "whitespace-only lines between content",
			input:    "Line1\n \nLine2\n\t\nLine3",
			expected: "Line1\n\nLine2\n\nLine3",
		},
		{
			name:     "consecutive whitespace-only lines",
			input:    "Line1\n \n\t\n  \nLine2",
			expected: "Line1\n\nLine2",
		},
		{
			name:     "alternating empty and whitespace lines",
			input:    "A\n \n\t\nB\n  \n \t \nC",
			expected: "A\n\nB\n\nC",
		},

		// Long whitespace
		{
			name:     "extremely long line of spaces",
			input:    "Start" + strings.Repeat(" ", 1000) + "End",
			expected: "Start End",
		},
		{
			name:     "extremely long line of mixed whitespace",
			input:    "Start" + strings.Repeat(" \t ", 200) + "End",
			expected: "Start End",
		},

		// Emoji tests
		{
			name:     "text with simple emojis",
			input:    "Hello  😀  world  👋  !",
			expected: "Hello 😀 world 👋 !",
		},
		{
			name:     "text with complex emojis",
			input:    "Family:  👨‍👩‍👧‍👦  Group:  👩‍👩‍👧‍👧  Work:  👨‍💻  Sports:  🏃‍♀️",
			expected: "Family: 👨‍👩‍👧‍👦 Group: 👩‍👩‍👧‍👧 Work: 👨‍💻 Sports: 🏃‍♀️",
		},
		{
			name:     "text with emoji modifiers",
			input:    "Skin tones:  👍  👍🏻  👍🏼  👍🏽  👍🏾  👍🏿",
			expected: "Skin tones: 👍 👍🏻 👍🏼 👍🏽 👍🏾 👍🏿",
		},
		{
			name:     "text with emoji and zero-width joiner",
			input:    "ZWJ sequences:  👨‍👩‍👧  👨‍❤️‍👨  👩‍❤️‍💋‍👩",
			expected: "ZWJ sequences: 👨‍👩‍👧 👨‍❤️‍👨 👩‍❤️‍💋‍👩",
		},
		{
			name:     "text with regional indicator symbols",
			input:    "Flags:  🇺🇸  🇯🇵  🇪🇺  🇬🇧  🇨🇦",
			expected: "Flags: 🇺🇸 🇯🇵 🇪🇺 🇬🇧 🇨🇦",
		},

		// Internationalization
		{
			name:     "text with accented characters",
			input:    "  résumé  \n  café  \n  naïve  ",
			expected: "résumé\ncafé\nnaïve",
		},
		{
			name:     "text with non-latin characters",
			input:    "  こんにちは  \n  世界  \n  你好  \n  세계  ",
			expected: "こんにちは\n世界\n你好\n세계",
		},
		{
			name:     "mixed language text",
			input:    "  Hello  世界  !  こんにちは  world  !  ",
			expected: "Hello 世界 ! こんにちは world !",
		},
		{
			name:     "right-to-left text",
			input:    "  مرحبا  بالعالم  \n  שלום  עולם  ",
			expected: "مرحبا بالعالم\nשלום עולם",
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel() // run subtests in parallel
			got := common.NormalizeText(tt.input)
			if got != tt.expected {
				t.Errorf("\nname: %s\ninput: %q\nwant: %q\ngot:  %q", tt.name, tt.input, tt.expected, got)
			}
		})
	}
}
