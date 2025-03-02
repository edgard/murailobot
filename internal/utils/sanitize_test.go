package utils_test

import (
	"testing"

	"github.com/edgard/murailobot/internal/utils"
)

func TestSanitizePlaintext(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic text handling
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "simple text",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "text with special characters",
			input:    "hello & world",
			expected: "hello & world",
		},
		{
			name:     "text with numeric and punctuation characters",
			input:    "  1234, 5678!  \n  $9.99  ",
			expected: "1234, 5678!\n$9.99",
		},

		// Whitespace handling
		{
			name:     "text with multiple spaces",
			input:    "hello   world",
			expected: "hello world",
		},
		{
			name:     "text with leading and trailing spaces",
			input:    "  hello world  ",
			expected: "hello world",
		},
		{
			name:     "text with tabs",
			input:    "hello\tworld\ttest",
			expected: "hello world test",
		},
		{
			name:     "text with non-breaking spaces",
			input:    "hello\u00A0\u00A0world",
			expected: "hello world",
		},
		{
			name:     "text with only whitespace",
			input:    "   \t\n\r\f\v   ",
			expected: "",
		},
		{
			name:     "text with unusual spacing",
			input:    "word\u205Fword\u2060word\u180Eword",
			expected: "word wordwordword",
		},

		// Newline handling
		{
			name:     "text with newline characters",
			input:    "hello\nworld",
			expected: "hello\nworld",
		},
		{
			name:     "text with two newline characters",
			input:    "hello\n\nworld",
			expected: "hello\n\nworld",
		},
		{
			name:     "text with three newline characters",
			input:    "hello\n\n\nworld",
			expected: "hello\n\nworld",
		},
		{
			name:     "text with four newline characters",
			input:    "hello\n\n\n\nworld",
			expected: "hello\n\nworld",
		},
		{
			name:     "text with very long newline sequence",
			input:    "hello\n\n\n\n\n\n\nworld",
			expected: "hello\n\nworld",
		},
		{
			name:     "text with multiple newline characters and spaces",
			input:    "hello \n \n world",
			expected: "hello\n\nworld",
		},
		{
			name:     "text with mixed spaces and newlines",
			input:    "  hello\n  world  ",
			expected: "hello\nworld",
		},
		{
			name:     "text with a lot of spaces and newlines",
			input:    "  hello   \n\n\n  world  ",
			expected: "hello\n\nworld",
		},
		{
			name:     "text with multiple spaces at line breaks",
			input:    "hello    \n    world",
			expected: "hello\nworld",
		},
		{
			name:     "text with special whitespace sequences",
			input:    "hello\n \n \n \nworld",
			expected: "hello\n\nworld",
		},

		// Line separators
		{
			name:     "text with carriage returns",
			input:    "hello\rworld",
			expected: "hello\nworld",
		},
		{
			name:     "text with mixed line endings",
			input:    "line1\rline2\r\nline3\nline4",
			expected: "line1\nline2\nline3\nline4",
		},
		{
			name:     "text with line separator",
			input:    "hello\u2028world",
			expected: "hello\nworld",
		},
		{
			name:     "text with paragraph separator",
			input:    "hello\u2029world",
			expected: "hello\n\nworld",
		},

		// Unicode and special characters
		{
			name:     "text with unicode characters",
			input:    "你好，世界",
			expected: "你好，世界",
		},
		{
			name:     "text with emoji",
			input:    "hello 👋 world 🌍",
			expected: "hello 👋 world 🌍",
		},
		{
			name:     "text with multiple languages",
			input:    "English 日本語 Español Français   العربية",
			expected: "English 日本語 Español Français العربية",
		},
		{
			name:     "text with fullwidth characters",
			input:    "ｈｅｌｌｏ　ｗｏｒｌｄ",
			expected: "ｈｅｌｌｏ　ｗｏｒｌｄ",
		},
		{
			name:     "text with combining diacritical marks",
			input:    "n\u0303o\u0308",
			expected: "n\u0303o\u0308",
		},
		{
			name:     "text with mathematical notation",
			input:    "x² + y² = z²   and   α + β = γ",
			expected: "x² + y² = z² and α + β = γ",
		},
		{
			name:     "text with quotes and apostrophes",
			input:    "'Single quotes' and \"Double quotes\" and \"Curly quotes\"",
			expected: "'Single quotes' and \"Double quotes\" and \"Curly quotes\"",
		},

		// Unicode control characters
		{
			name:     "text with zero-width spaces",
			input:    "hello\u200Bworld\u200B",
			expected: "hello world",
		},
		{
			name:     "text with zero-width non-joiner",
			input:    "hello\u200Cworld",
			expected: "hello world",
		},
		{
			name:     "text with bidirectional characters",
			input:    "Hello \u202Eworld\u202C test",
			expected: "Hello world test",
		},
		{
			name:     "text with control characters",
			input:    "hello\u0000\u0001\u0002world",
			expected: "hello world",
		},
		{
			name:     "text with byte order mark",
			input:    "\uFEFFHello world",
			expected: "Hello world",
		},
		{
			name:     "text with soft hyphens",
			input:    "super\u00ADcalifragilistic",
			expected: "supercalifragilistic",
		},
		{
			name:     "text with Unicode joiners",
			input:    "zero\u2060width\u2060joiner",
			expected: "zerowidthjoiner",
		},

		// Mixed character handling
		{
			name:     "text with mixed whitespace characters",
			input:    "hello\t \r\nworld\f\vtest",
			expected: "hello\nworld test",
		},
		{
			name:     "text with multiple consecutive whitespace types",
			input:    "hello\r\n\t \f\vworld",
			expected: "hello\nworld",
		},

		// Specific content handling
		{
			name:     "text with URLs",
			input:    "Visit https://example.com/test?param=value",
			expected: "Visit https://example.com/test?param=value",
		},
		{
			name:     "text with email addresses",
			input:    "Contact user@example.com   for support",
			expected: "Contact user@example.com for support",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := utils.Sanitize(tc.input)

			if actual != tc.expected {
				t.Errorf("input: %q, expected: %q, actual: %q", tc.input, tc.expected, actual)
			}
		})
	}
}

func TestSanitizeMarkdown(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "text with bold markdown",
			input:    "This is **bold text** and this is __also bold__",
			expected: "This is bold text and this is also bold",
		},
		{
			name:     "text with italic markdown",
			input:    "This is *italic text* and this is _also italic_",
			expected: "This is italic text and this is also italic",
		},
		{
			name:     "text with markdown links",
			input:    "Visit [this website](https://example.com) for more information",
			expected: "Visit this website (https://example.com) for more information",
		},
		{
			name:     "text with markdown images",
			input:    "Here's an image: ![alt text](https://example.com/image.jpg)",
			expected: "Here's an image: https://example.com/image.jpg",
		},
		{
			name:     "text with markdown headers",
			input:    "# Main Title\n## Subtitle\nRegular text here",
			expected: "Main Title\nSubtitle\nRegular text here",
		},
		{
			name:     "text with markdown blockquotes",
			input:    "Here's a quote:\n> This is quoted\n> On multiple lines",
			expected: "Here's a quote:\nThis is quoted\nOn multiple lines",
		},
		{
			name:     "text with markdown code blocks",
			input:    "Code example:\n```go\nfunc main() {\n    fmt.Println(\"Hello world\")\n}\n```",
			expected: "Code example:",
		},
		{
			name:     "text with inline markdown code",
			input:    "Use the `print()` function to output text",
			expected: "Use the function to output text",
		},
		{
			name:     "text with markdown lists",
			input:    "Shopping list:\n* Apples\n* Bananas\n* Oranges\n\nSteps:\n1. Plan\n2. Execute",
			expected: "Shopping list:\nApples\nBananas\nOranges\n\nSteps:\nPlan\nExecute",
		},
		{
			name:     "text with horizontal rules",
			input:    "Above\n---\nBelow\n***\nEnd",
			expected: "Above\nBelow\nEnd",
		},
		{
			name:     "text with complex markdown",
			input:    "# Project README\n\n## Features\n* **Bold item** with _emphasis_\n* [Link](https://example.com)\n\n```\nSample code\n```\n\n> Note: This is important",
			expected: "Project README\n\nFeatures\nBold item with emphasis\nLink (https://example.com)\n\nNote: This is important",
		},
		{
			name:     "text with markdown strikethrough",
			input:    "This is ~~deleted~~ text",
			expected: "This is deleted text",
		},
		{
			name:     "text with mixed markdown and whitespace",
			input:    "  **Bold**  \n  _Italic_  \n\n\n> Quote",
			expected: "Bold\nItalic\n\nQuote",
		},
		{
			name:     "text with nested markdown formatting",
			input:    "This is **bold _and italic_** text",
			expected: "This is bold and italic text",
		},
		{
			name:     "text with markdown tables",
			input:    "| Header 1 | Header 2 |\n| -------- | -------- |\n| Cell 1   | Cell 2   |",
			expected: "Header 1 Header 2\nCell 1 Cell 2",
		},
		{
			name:     "text with escaped markdown",
			input:    "This \\*is not italic\\* and this \\`is not code\\`",
			expected: "This *is not italic* and this `is not code`",
		},
		{
			name:     "markdown link with same text and url",
			input:    "Click here: [https://example.com](https://example.com)",
			expected: "Click here: https://example.com",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := utils.Sanitize(tc.input)

			if actual != tc.expected {
				t.Errorf("input: %q, expected: %q, actual: %q", tc.input, tc.expected, actual)
			}
		})
	}
}

func TestIsMarkdown(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "plain text",
			input:    "This is just regular text",
			expected: false,
		},
		{
			name:     "bold markdown",
			input:    "This is **bold text**",
			expected: true,
		},
		{
			name:     "italic markdown",
			input:    "This is *italic text*",
			expected: true,
		},
		{
			name:     "link markdown",
			input:    "[link text](https://example.com)",
			expected: true,
		},
		{
			name:     "header markdown",
			input:    "# Header",
			expected: true,
		},
		{
			name:     "code block",
			input:    "```\ncode\n```",
			expected: true,
		},
		{
			name:     "blockquote",
			input:    "> quoted text",
			expected: true,
		},
		{
			name:     "escaped markdown",
			input:    "This \\*is not markdown\\*",
			expected: false,
		},
		{
			name:     "setext header markdown",
			input:    "Header Text\n---",
			expected: true,
		},
		{
			name:     "task list markdown detection",
			input:    "- [ ] Incomplete task",
			expected: true,
		},
		{
			name:     "footnote markdown detection",
			input:    "[^1]: This is a footnote",
			expected: true,
		},
		{
			name:     "indented code block detection",
			input:    "    func main() { }",
			expected: true,
		},
		{
			name:     "inline math detection",
			input:    "The equation is $E=mc^2$ which is famous.",
			expected: true,
		},
		{
			name:     "display math detection",
			input:    "$$\nE=mc^2\n$$",
			expected: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := utils.IsMarkdown(tc.input)

			if actual != tc.expected {
				t.Errorf("IsMarkdown(%q) = %v, expected %v", tc.input, actual, tc.expected)
			}
		})
	}
}
