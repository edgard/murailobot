package openai

import (
	"strings"
	"testing"

	"go.uber.org/zap"
)

// TestSanitize tests the aiService.Sanitize method with various input types.
func TestSanitize(t *testing.T) {
	t.Parallel()

	// Create a minimal aiService for testing
	logger, _ := zap.NewDevelopment()
	service := &aiService{
		logger: logger,
	}

	// Define test cases
	tests := []struct {
		group    string
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		// Basic Input Validation
		{"Basic Input", "empty string", "", "", true},
		{"Basic Input", "simple text", "hello world", "hello world", false},
		{"Basic Input", "only whitespace", "   \t\n\r\f\v   ", "", true},
		{"Basic Input", "single character", "a", "a", false},
		{"Basic Input", "single space", " ", "", true},
		{"Basic Input", "single newline", "\n", "", true},
		{"Basic Input", "single tab", "\t", "", true},
		{"Basic Input", "single return", "\r", "", true},
		{"Basic Input", "alphanumeric characters", "abcdef123456", "abcdef123456", false},
		{"Basic Input", "long string", strings.Repeat("x", 1000), strings.Repeat("x", 1000), false},

		// Metadata Removal
		{"Metadata", "No metadata", "This is a normal response without metadata.", "This is a normal response without metadata.", false},
		{"Metadata", "With metadata at beginning", "[2025-03-06T22:30:11+01:00] UID 123456 (@username): This is a response with metadata.", "This is a response with metadata.", false},
		{"Metadata", "With metadata and whitespace", "  [2025-03-06T22:30:11+01:00] UID 123456 (User Name):   The actual response content.", "The actual response content.", false},
		{"Metadata", "With metadata in UTC format", "[2025-03-06T21:30:11Z] UID 123456 (unknown): Response with UTC timestamp.", "Response with UTC timestamp.", false},
		{"Metadata", "With metadata and multiple lines", "[2025-03-06T22:30:11+01:00] UID 123456 (@username): First line.\nSecond line.\nThird line.", "First line.\nSecond line.\nThird line.", false},
		{"Metadata", "With metadata in middle of text (should not remove)", "This text has [2025-03-06T22:30:11+01:00] UID 123456 (@username): in the middle.", "This text has [2025-03-06T22:30:11+01:00] UID 123456 (@username): in the middle.", false},
		{"Metadata", "Only metadata", "[2025-03-06T23:19:51+01:00] BOT:", "", true},
		{"Metadata", "With metadata with fractional seconds", "[2025-03-06T22:30:11.123+01:00] UID 123456 (@username): Response with fractional seconds.", "Response with fractional seconds.", false},
		{"Metadata", "With multiple colons in identifier", "[2025-03-06T22:30:11+01:00] System:Log:Info: The message content", "The message content", false},
		{"Metadata", "With emoji in metadata", "[2025-03-06T22:30:11+01:00] UID 123456 (😊): Message with emoji", "Message with emoji", false},

		// Whitespace Handling
		{"Whitespace", "text with multiple spaces", "hello   world", "hello world", false},
		{"Whitespace", "text with leading and trailing spaces", "  hello world  ", "hello world", false},
		{"Whitespace", "text with tabs", "hello\tworld\ttest", "hello world test", false},
		{"Whitespace", "text with mixed whitespace", "hello \t \r\n world", "hello\nworld", false},
		{"Whitespace", "text with consecutive spaces between words", "hello      world      test", "hello world test", false},
		{"Whitespace", "text with spaces before punctuation", "hello   , world   !", "hello, world!", false},
		{"Whitespace", "text with spaces after punctuation", "hello,   world!   ", "hello, world!", false},
		{"Whitespace", "text with spaces around brackets", "hello   (   world   )   !", "hello ( world ) !", false},
		{"Whitespace", "text with many consecutive spaces", "hello" + strings.Repeat(" ", 100) + "world", "hello world", false},
		{"Whitespace", "text with non-breaking spaces", "hello\u00A0\u00A0world", "hello world", false},

		// Newline Handling
		{"Newlines", "text with newline characters", "hello\nworld", "hello\nworld", false},
		{"Newlines", "text with two newline characters", "hello\n\nworld", "hello\n\nworld", false},
		{"Newlines", "text with three newline characters", "hello\n\n\nworld", "hello\n\nworld", false},
		{"Newlines", "text with many newline characters", "hello\n\n\n\n\n\nworld", "hello\n\nworld", false},
		{"Newlines", "text with newlines and whitespace", "  hello\n  \n  world  ", "hello\n\nworld", false},
		{"Newlines", "only newlines", "\n\n\n\n", "", true},
		{"Newlines", "text with newlines at beginning", "\n\n\nhello world", "hello world", false},
		{"Newlines", "text with newlines at end", "hello world\n\n\n", "hello world", false},
		{"Newlines", "complex newline pattern", "Line1\n\nLine2\nLine3\n\n\n\nLine4\n\nLine5", "Line1\n\nLine2\nLine3\n\nLine4\n\nLine5", false},
		{"Newlines", "paragraphs with excessive spacing", "Paragraph 1.\n\n\n\nParagraph 2.\n\n\n\nParagraph 3.", "Paragraph 1.\n\nParagraph 2.\n\nParagraph 3.", false},

		// Line Ending Normalization
		{"Line Endings", "text with carriage returns", "hello\rworld", "hello\nworld", false},
		{"Line Endings", "text with mixed line endings", "line1\rline2\r\nline3\nline4", "line1\nline2\nline3\nline4", false},
		{"Line Endings", "text with all CRLF line endings", "line1\r\nline2\r\nline3\r\nline4", "line1\nline2\nline3\nline4", false},
		{"Line Endings", "text with all CR line endings", "line1\rline2\rline3\rline4", "line1\nline2\nline3\nline4", false},
		{"Line Endings", "text with consecutive CR line endings", "line1\r\rline2", "line1\n\nline2", false},
		{"Line Endings", "Windows formatted text", "This is line 1.\r\nThis is line 2.\r\n\r\nThis is a new paragraph.", "This is line 1.\nThis is line 2.\n\nThis is a new paragraph.", false},
		{"Line Endings", "Old Mac formatted text", "This is line 1.\rThis is line 2.\r\rThis is a new paragraph.", "This is line 1.\nThis is line 2.\n\nThis is a new paragraph.", false},
		{"Line Endings", "Mixed format document with whitespace", "Line 1\r\n  Line 2\rLine 3\n\r\nLine 4", "Line 1\nLine 2\nLine 3\n\nLine 4", false},
		{"Line Endings", "Multiple CRs only", "\r\r\r\r", "", true},
		{"Line Endings", "Multiple CRLFs only", "\r\n\r\n\r\n", "", true},

		// Multiline Content
		{"Multiline", "text with code snippets", "```javascript\nfunction hello() {\n  console.log('world');\n}\n```", "```javascript\nfunction hello() {\nconsole.log('world');\n}\n```", false},
		{"Multiline", "text with complex formatting", "# Title\n\n## Subtitle\n\n- List item 1\n- List item 2\n\n> Blockquote\n\nParagraph", "# Title\n\n## Subtitle\n\n- List item 1\n- List item 2\n\n> Blockquote\n\nParagraph", false},
		{"Multiline", "markdown with code blocks and indentation", "# Code Example\n\n```python\ndef hello():\n    print('Hello, world!')\n    return True\n```\n\nResult: `True`", "# Code Example\n\n```python\ndef hello():\nprint('Hello, world!')\nreturn True\n```\n\nResult: `True`", false},
		{"Multiline", "JSON structure", "{\n  \"name\": \"John\",\n  \"age\": 30,\n  \"city\": \"New York\"\n}", "{\n\"name\": \"John\",\n\"age\": 30,\n\"city\": \"New York\"\n}", false},
		{"Multiline", "code with trailing whitespace on each line", "function hello() {   \n  console.log('world');   \n}   ", "function hello() {\nconsole.log('world');\n}", false},

		// Real-world Examples
		{"Real-world", "text with URLs", "Visit https://example.com/test?param=value", "Visit https://example.com/test?param=value", false},
		{"Real-world", "text with email addresses", "Contact user@example.com   for support", "Contact user@example.com for support", false},
		{"Real-world", "text with special characters", "Hello & world! This costs $9.99 + tax", "Hello & world! This costs $9.99 + tax", false},
		{"Real-world", "technical error message", "Error: Cannot connect to database (code: DB_CONN_REFUSED)\nDetails: Connection timed out after 30s", "Error: Cannot connect to database (code: DB_CONN_REFUSED)\nDetails: Connection timed out after 30s", false},
		{"Real-world", "text with SQL query", "Try this query: SELECT * FROM users WHERE age > 18 ORDER BY name;", "Try this query: SELECT * FROM users WHERE age > 18 ORDER BY name;", false},
		{"Real-world", "text with file paths", "Save the file to C:\\Users\\John\\Documents\\report.pdf or /home/user/documents/report.pdf", "Save the file to C:\\Users\\John\\Documents\\report.pdf or /home/user/documents/report.pdf", false},
		{"Real-world", "text with quotes and apostrophes", "She said, \"That's not what I meant.\" We weren't sure what to do.", "She said, \"That's not what I meant.\" We weren't sure what to do.", false},

		// Emoji Handling
		{"Emoji", "basic emoji", "Hello 😊 world", "Hello 😊 world", false},
		{"Emoji", "multiple emojis", "Hello 😊 👋 🌎 world", "Hello 😊 👋 🌎 world", false},
		{"Emoji", "emojis with whitespace", "Hello   😊   world", "Hello 😊 world", false},
		{"Emoji", "only emoji", "😊", "😊", false},
		{"Emoji", "complex emoji (ZWJ sequence)", "Family: 👨‍👩‍👧‍👦", "Family: 👨‍👩‍👧‍👦", false},
		{"Emoji", "skin tone modifier", "Hi 👋🏽 world", "Hi 👋🏽 world", false},
		{"Emoji", "flag emoji", "Countries: 🇧🇷 🇯🇵 🇺🇸 🇪🇺", "Countries: 🇧🇷 🇯🇵 🇺🇸 🇪🇺", false},
		{"Emoji", "emoji with metadata and extra whitespace", "[2025-03-06T22:30:11Z] UID 123 (@user):    Hello   🌍  !  ", "Hello 🌍 !", false},
		{"Emoji", "emoji with presentation selector", "Watch: ⌚️ vs text watch: ⌚", "Watch: ⌚️ vs text watch: ⌚", false},
		{"Emoji", "emojis with invisible joins", "Welcome to 🏳️‍⚧️ community and the 👨🏾‍🦱 club", "Welcome to 🏳️‍⚧️ community and the 👨🏾‍🦱 club", false},
	}

	// Run tests directly, organized by group using subtests
	for _, tc := range tests {
		tc := tc // Capture for closure
		testName := tc.group + " / " + tc.name

		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			actual, err := service.Sanitize(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("Sanitize() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if !tc.wantErr && actual != tc.expected {
				t.Errorf("input: %q, expected: %q, actual: %q", tc.input, tc.expected, actual)
			}
		})
	}
}
