package openai

import (
	"strings"
	"testing"

	"go.uber.org/zap"
)

// TestSanitize tests the aiService.Sanitize method with various input types.
// The tests are organized into logical categories using subtests.
func TestSanitize(t *testing.T) {
	t.Parallel()

	// Create a minimal aiService for testing
	logger, _ := zap.NewDevelopment()
	service := &aiService{
		logger: logger,
	}

	// Define a common test case structure for all Sanitize tests
	type sanitizeTestCase struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}

	// Group test cases by functionality for better organization
	testGroups := map[string][]sanitizeTestCase{
		"Basic Input Validation": {
			{
				name:     "empty string",
				input:    "",
				expected: "",
				wantErr:  true,
			},
			{
				name:     "simple text",
				input:    "hello world",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "only whitespace",
				input:    "   \t\n\r\f\v   ",
				expected: "",
				wantErr:  true,
			},
			{
				name:     "single character",
				input:    "a",
				expected: "a",
				wantErr:  false,
			},
			{
				name:     "single space",
				input:    " ",
				expected: "",
				wantErr:  true,
			},
			{
				name:     "single newline",
				input:    "\n",
				expected: "",
				wantErr:  true,
			},
			{
				name:     "single tab",
				input:    "\t",
				expected: "",
				wantErr:  true,
			},
			{
				name:     "single return",
				input:    "\r",
				expected: "",
				wantErr:  true,
			},
			{
				name:     "alphanumeric characters",
				input:    "abcdef123456",
				expected: "abcdef123456",
				wantErr:  false,
			},
			{
				name:     "punctuation only",
				input:    "!@#$%^&*()",
				expected: "!@#$%^&*()",
				wantErr:  false,
			},
			{
				name:     "mixed alphanumeric and punctuation",
				input:    "abc123!@#",
				expected: "abc123!@#",
				wantErr:  false,
			},
			{
				name:     "numeric only",
				input:    "123456",
				expected: "123456",
				wantErr:  false,
			},
			{
				name:     "long string",
				input:    strings.Repeat("x", 1000),
				expected: strings.Repeat("x", 1000),
				wantErr:  false,
			},
			{
				name:     "mostly whitespace with one character",
				input:    "   \t\n\r\f\v  x \t\n\r\f\v   ",
				expected: "x",
				wantErr:  false,
			},
			{
				name:     "single zero-width space character",
				input:    "\u200B",
				expected: "",
				wantErr:  true,
			},
			{
				name:     "text with zero-width space character",
				input:    "hello\u200Bworld",
				expected: "helloworld",
				wantErr:  false,
			},
			{
				name:     "just multiple spaces and a non-breaking space",
				input:    "  \u00A0  ",
				expected: "",
				wantErr:  true,
			},
			{
				name:     "non-breaking space between words",
				input:    "hello\u00A0world",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text surrounded by zero-width spaces",
				input:    "\u200Bhello world\u200B",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "just a period",
				input:    ".",
				expected: ".",
				wantErr:  false,
			},
		},
		"Metadata Removal": {
			{
				name:     "No metadata",
				input:    "This is a normal response without metadata.",
				expected: "This is a normal response without metadata.",
				wantErr:  false,
			},
			{
				name:     "With metadata at beginning",
				input:    "[2025-03-06T22:30:11+01:00] UID 123456 (@username): This is a response with metadata.",
				expected: "This is a response with metadata.",
				wantErr:  false,
			},
			{
				name:     "With metadata and whitespace",
				input:    "  [2025-03-06T22:30:11+01:00] UID 123456 (User Name):   The actual response content.",
				expected: "The actual response content.",
				wantErr:  false,
			},
			{
				name:     "With metadata in UTC format",
				input:    "[2025-03-06T21:30:11Z] UID 123456 (unknown): Response with UTC timestamp.",
				expected: "Response with UTC timestamp.",
				wantErr:  false,
			},
			{
				name:     "With metadata and multiple lines",
				input:    "[2025-03-06T22:30:11+01:00] UID 123456 (@username): First line.\nSecond line.\nThird line.",
				expected: "First line.\nSecond line.\nThird line.",
				wantErr:  false,
			},
			{
				name:     "With metadata in middle of text (should not remove)",
				input:    "This text has [2025-03-06T22:30:11+01:00] UID 123456 (@username): in the middle.",
				expected: "This text has [2025-03-06T22:30:11+01:00] UID 123456 (@username): in the middle.",
				wantErr:  false,
			},
			{
				name:     "With metadata with fractional seconds",
				input:    "[2025-03-06T22:30:11.123+01:00] UID 123456 (@username): Response with fractional seconds.",
				expected: "Response with fractional seconds.",
				wantErr:  false,
			},
			{
				name:     "With multiple colons in identifier",
				input:    "[2025-03-06T22:30:11+01:00] System:Log:Info: The message content",
				expected: "The message content",
				wantErr:  false,
			},
			{
				name:     "Real reply metadata",
				input:    "[2025-03-06T23:19:51+01:00] BOT: coé beloiro, tá bolado porque tá chovendo na polonia? kkkkkkkk vai dormi mermão",
				expected: "coé beloiro, tá bolado porque tá chovendo na polonia? kkkkkkkk vai dormi mermão",
				wantErr:  false,
			},
			{
				name:     "Only metadata",
				input:    "[2025-03-06T23:19:51+01:00] BOT:",
				expected: "",
				wantErr:  true,
			},
			{
				name:     "With metadata and negative timezone",
				input:    "[2025-03-06T22:30:11-08:00] UID 123456 (@username): Response with negative timezone.",
				expected: "Response with negative timezone.",
				wantErr:  false,
			},
			{
				name:     "With metadata with different timestamp format (should not remove)",
				input:    "[06-03-2025 22:30:11] UID 123456: This should not be removed.",
				expected: "[06-03-2025 22:30:11] UID 123456: This should not be removed.",
				wantErr:  false,
			},
			{
				name:     "Metadata with numerical username",
				input:    "[2025-03-06T22:30:11+01:00] UID 123456 (12345): Response with numerical username.",
				expected: "Response with numerical username.",
				wantErr:  false,
			},
			{
				name:     "Metadata with no parentheses",
				input:    "[2025-03-06T22:30:11+01:00] System User: Message content only.",
				expected: "Message content only.",
				wantErr:  false,
			},
			{
				name:     "Metadata with high precision fractional seconds",
				input:    "[2025-03-06T22:30:11.123456789Z] UID 123456: Message with high precision time.",
				expected: "Message with high precision time.",
				wantErr:  false,
			},
			{
				name:     "Metadata with space after colon",
				input:    "[2025-03-06T22:30:11Z] User:  Content with extra space after colon.",
				expected: "Content with extra space after colon.",
				wantErr:  false,
			},
			{
				name:     "Metadata followed by empty line then content",
				input:    "[2025-03-06T22:30:11Z] Bot: \n\nContent after empty line.",
				expected: "\n\nContent after empty line.",
				wantErr:  false,
			},
			{
				name:     "Metadata with international characters in username",
				input:    "[2025-03-06T22:30:11Z] UID 123456 (Jürgen): Message with international username.",
				expected: "Message with international username.",
				wantErr:  false,
			},
			{
				name:     "Metadata at end (should not remove)",
				input:    "This message ends with metadata [2025-03-06T22:30:11Z] UID 123456:",
				expected: "This message ends with metadata [2025-03-06T22:30:11Z] UID 123456:",
				wantErr:  false,
			},
			{
				name:     "Metadata with empty message after colon and space",
				input:    "[2025-03-06T22:30:11Z] UID 123456: ",
				expected: "",
				wantErr:  true,
			},
		},
		"Whitespace Handling": {
			{
				name:     "text with multiple spaces",
				input:    "hello   world",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with leading and trailing spaces",
				input:    "  hello world  ",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with tabs",
				input:    "hello\tworld\ttest",
				expected: "hello world test",
				wantErr:  false,
			},
			{
				name:     "text with mixed whitespace",
				input:    "hello \t \r\n world",
				expected: "hello\nworld",
				wantErr:  false,
			},
			{
				name:     "text with consecutive spaces between words",
				input:    "hello      world      test",
				expected: "hello world test",
				wantErr:  false,
			},
			{
				name:     "text with spaces before punctuation",
				input:    "hello   , world   !",
				expected: "hello, world!",
				wantErr:  false,
			},
			{
				name:     "text with spaces after punctuation",
				input:    "hello,   world!   ",
				expected: "hello, world!",
				wantErr:  false,
			},
			{
				name:     "text with spaces around brackets",
				input:    "hello   (   world   )   !",
				expected: "hello ( world ) !",
				wantErr:  false,
			},
			{
				name:     "text with spaces and tabs mixed",
				input:    "hello  \t  world  \t  !",
				expected: "hello world !",
				wantErr:  false,
			},
			{
				name:     "text with non-breaking spaces",
				input:    "hello\u00A0\u00A0world",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with form feeds",
				input:    "hello\fworld",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with vertical tabs",
				input:    "hello\vworld",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with all types of whitespace",
				input:    "hello\f\v\r\n\t world",
				expected: "hello\nworld",
				wantErr:  false,
			},
			{
				name:     "text with zero-width spaces",
				input:    "hello\u200Bworld",
				expected: "helloworld",
				wantErr:  false,
			},
			{
				name:     "text with many consecutive spaces",
				input:    "hello" + strings.Repeat(" ", 100) + "world",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with indentation patterns",
				input:    "  • First item\n    • Nested item\n  • Second item",
				expected: "• First item\n• Nested item\n• Second item",
				wantErr:  false,
			},
			{
				name:     "text with various Unicode whitespace",
				input:    "hello\u2000world\u2001test\u2002example",
				expected: "hello world test example",
				wantErr:  false,
			},
			{
				name:     "text with whitespace between CJK characters",
				input:    "你好　世界", // Note: that's an ideographic space between 好 and 世
				expected: "你好 世界",
				wantErr:  false,
			},
			{
				name:     "text with multiple whitespace types between each word",
				input:    "word1 \t \f \v \r\nword2 \t \f \v \r\nword3",
				expected: "word1\nword2\nword3",
				wantErr:  false,
			},
			{
				name:     "text with whitespace in URLs (should preserve)",
				input:    "https://example.com/path with spaces/file.html",
				expected: "https://example.com/path with spaces/file.html",
				wantErr:  false,
			},
		},
		"Newline Handling": {
			{
				name:     "text with newline characters",
				input:    "hello\nworld",
				expected: "hello\nworld",
				wantErr:  false,
			},
			{
				name:     "text with two newline characters",
				input:    "hello\n\nworld",
				expected: "hello\n\nworld",
				wantErr:  false,
			},
			{
				name:     "text with three newline characters",
				input:    "hello\n\n\nworld",
				expected: "hello\n\nworld",
				wantErr:  false,
			},
			{
				name:     "text with many newline characters",
				input:    "hello\n\n\n\n\n\nworld",
				expected: "hello\n\nworld",
				wantErr:  false,
			},
			{
				name:     "text with newlines and whitespace",
				input:    "  hello\n  \n  world  ",
				expected: "hello\n\nworld",
				wantErr:  false,
			},
			{
				name:     "only newlines",
				input:    "\n\n\n\n",
				expected: "",
				wantErr:  true,
			},
			{
				name:     "text with newlines at beginning",
				input:    "\n\n\nhello world",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with newlines at end",
				input:    "hello world\n\n\n",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with newlines and content on each line",
				input:    "first\nsecond\nthird\nfourth",
				expected: "first\nsecond\nthird\nfourth",
				wantErr:  false,
			},
			{
				name:     "text with alternating content and multiple newlines",
				input:    "first\n\n\nsecond\n\n\nthird",
				expected: "first\n\nsecond\n\nthird",
				wantErr:  false,
			},
			{
				name:     "text with newlines and spaces on empty lines",
				input:    "hello\n   \n   \nworld",
				expected: "hello\n\nworld",
				wantErr:  false,
			},
			{
				name:     "text with excessive newlines at beginning and end",
				input:    "\n\n\n\nhello\nworld\n\n\n\n",
				expected: "hello\nworld",
				wantErr:  false,
			},
			{
				name:     "text with mixed whitespace creating newlines",
				input:    "hello\r\n\r\nworld",
				expected: "hello\n\nworld",
				wantErr:  false,
			},
			{
				name:     "paragraphs with proper spacing",
				input:    "Paragraph 1.\n\nParagraph 2.\n\nParagraph 3.",
				expected: "Paragraph 1.\n\nParagraph 2.\n\nParagraph 3.",
				wantErr:  false,
			},
			{
				name:     "paragraphs with excessive spacing",
				input:    "Paragraph 1.\n\n\n\nParagraph 2.\n\n\n\nParagraph 3.",
				expected: "Paragraph 1.\n\nParagraph 2.\n\nParagraph 3.",
				wantErr:  false,
			},
			{
				name:     "text with many newlines in the middle",
				input:    "start" + strings.Repeat("\n", 50) + "end",
				expected: "start\n\nend",
				wantErr:  false,
			},
			{
				name:     "newlines with tabs and spaces between",
				input:    "hello\n \t \n \t \nworld",
				expected: "hello\n\nworld",
				wantErr:  false,
			},
			{
				name:     "complex newline pattern",
				input:    "Line1\n\nLine2\nLine3\n\n\n\nLine4\n\nLine5",
				expected: "Line1\n\nLine2\nLine3\n\nLine4\n\nLine5",
				wantErr:  false,
			},
			{
				name:     "single newline between paragraphs (should preserve)",
				input:    "Paragraph 1.\nParagraph 2.\nParagraph 3.",
				expected: "Paragraph 1.\nParagraph 2.\nParagraph 3.",
				wantErr:  false,
			},
			{
				name:     "newlines in code blocks (should preserve structure but normalize whitespace)",
				input:    "```\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```",
				expected: "```\nfunc main() {\nfmt.Println(\"Hello\")\n}\n```",
				wantErr:  false,
			},
		},
		"Line Ending Normalization": {
			{
				name:     "text with carriage returns",
				input:    "hello\rworld",
				expected: "hello\nworld",
				wantErr:  false,
			},
			{
				name:     "text with mixed line endings",
				input:    "line1\rline2\r\nline3\nline4",
				expected: "line1\nline2\nline3\nline4",
				wantErr:  false,
			},
			{
				name:     "text with all CRLF line endings",
				input:    "line1\r\nline2\r\nline3\r\nline4",
				expected: "line1\nline2\nline3\nline4",
				wantErr:  false,
			},
			{
				name:     "text with all CR line endings",
				input:    "line1\rline2\rline3\rline4",
				expected: "line1\nline2\nline3\nline4",
				wantErr:  false,
			},
			{
				name:     "text with consecutive CR line endings",
				input:    "line1\r\rline2",
				expected: "line1\n\nline2",
				wantErr:  false,
			},
			{
				name:     "text with consecutive CRLF line endings",
				input:    "line1\r\n\r\nline2",
				expected: "line1\n\nline2",
				wantErr:  false,
			},
			{
				name:     "text with mixed consecutive line endings",
				input:    "line1\r\n\rline2",
				expected: "line1\n\nline2",
				wantErr:  false,
			},
			{
				name:     "text with CR at end",
				input:    "hello world\r",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with CRLF at end",
				input:    "hello world\r\n",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with CR at start",
				input:    "\rhello world",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with CRLF at start",
				input:    "\r\nhello world",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with multiple consecutive mixed line endings",
				input:    "line1\r\n\r\r\n\r\nline2",
				expected: "line1\n\nline2",
				wantErr:  false,
			},
			{
				name:     "Windows formatted text (CRLF throughout)",
				input:    "This is line 1.\r\nThis is line 2.\r\n\r\nThis is a new paragraph.",
				expected: "This is line 1.\nThis is line 2.\n\nThis is a new paragraph.",
				wantErr:  false,
			},
			{
				name:     "Old Mac formatted text (CR throughout)",
				input:    "This is line 1.\rThis is line 2.\r\rThis is a new paragraph.",
				expected: "This is line 1.\nThis is line 2.\n\nThis is a new paragraph.",
				wantErr:  false,
			},
			{
				name:     "Mixed format document with whitespace",
				input:    "Line 1\r\n  Line 2\rLine 3\n\r\nLine 4",
				expected: "Line 1\nLine 2\nLine 3\n\nLine 4",
				wantErr:  false,
			},
			{
				name:     "CR inside a word (should normalize but keep as one word)",
				input:    "Hel\rlo",
				expected: "Hel\nlo",
				wantErr:  false,
			},
			{
				name:     "CRLF inside a word (should normalize but keep as one word)",
				input:    "Hel\r\nlo",
				expected: "Hel\nlo",
				wantErr:  false,
			},
			{
				name:     "Multiple CRs only",
				input:    "\r\r\r\r",
				expected: "",
				wantErr:  true,
			},
			{
				name:     "Multiple CRLFs only",
				input:    "\r\n\r\n\r\n",
				expected: "",
				wantErr:  true,
			},
			{
				name:     "Complex nested line endings",
				input:    "Line1\r\n\rLine2\n\r\n\rLine3",
				expected: "Line1\n\nLine2\n\nLine3",
				wantErr:  false,
			},
		},
		"Multiline Content": {
			{
				name:     "text with code snippets",
				input:    "```javascript\nfunction hello() {\n  console.log('world');\n}\n```",
				expected: "```javascript\nfunction hello() {\nconsole.log('world');\n}\n```",
				wantErr:  false,
			},
			{
				name:     "text with complex formatting",
				input:    "# Title\n\n## Subtitle\n\n- List item 1\n- List item 2\n\n> Blockquote\n\nParagraph",
				expected: "# Title\n\n## Subtitle\n\n- List item 1\n- List item 2\n\n> Blockquote\n\nParagraph",
				wantErr:  false,
			},
			{
				name:     "markdown with code blocks and indentation",
				input:    "# Code Example\n\n```python\ndef hello():\n    print('Hello, world!')\n    return True\n```\n\nResult: `True`",
				expected: "# Code Example\n\n```python\ndef hello():\nprint('Hello, world!')\nreturn True\n```\n\nResult: `True`",
				wantErr:  false,
			},
			{
				name:     "markdown with nested lists",
				input:    "# Shopping List\n\n- Fruits\n  - Apples\n  - Bananas\n- Vegetables\n  - Carrots\n  - Broccoli",
				expected: "# Shopping List\n\n- Fruits\n- Apples\n- Bananas\n- Vegetables\n- Carrots\n- Broccoli",
				wantErr:  false,
			},
			{
				name:     "markdown with tables",
				input:    "| Name | Age | City |\n|------|-----|------|\n| John | 30  | NY   |\n| Mary | 25  | LA   |",
				expected: "| Name | Age | City |\n|------|-----|------|\n| John | 30 | NY |\n| Mary | 25 | LA |",
				wantErr:  false,
			},
			{
				name:     "markdown with blockquotes",
				input:    "> This is a quote\n> Second line of quote\n\nNormal text",
				expected: "> This is a quote\n> Second line of quote\n\nNormal text",
				wantErr:  false,
			},
			{
				name:     "markdown with horizontal rules",
				input:    "Above\n\n---\n\nBelow",
				expected: "Above\n\n---\n\nBelow",
				wantErr:  false,
			},
			{
				name:     "markdown with inline formatting",
				input:    "This is **bold**, *italic*, and `code`.",
				expected: "This is **bold**, *italic*, and `code`.",
				wantErr:  false,
			},
			{
				name:     "markdown with links",
				input:    "Check [this link](https://example.com) and [that one](https://example.org).",
				expected: "Check [this link](https://example.com) and [that one](https://example.org).",
				wantErr:  false,
			},
			{
				name:     "markdown with images",
				input:    "![Alt text](image.jpg \"Optional title\")",
				expected: "![Alt text](image.jpg \"Optional title\")",
				wantErr:  false,
			},
			{
				name:     "multiline text with excessive whitespace",
				input:    "Line 1   \n   Line 2   \n\n   Line 3",
				expected: "Line 1\nLine 2\n\nLine 3",
				wantErr:  false,
			},
			{
				name:     "multiline with mixed formatting",
				input:    "# Header\n\n* Bullet 1\n* Bullet 2\n\n```\ncode block\n```\n\n> Quote",
				expected: "# Header\n\n* Bullet 1\n* Bullet 2\n\n```\ncode block\n```\n\n> Quote",
				wantErr:  false,
			},
			{
				name:     "JSON structure",
				input:    "{\n  \"name\": \"John\",\n  \"age\": 30,\n  \"city\": \"New York\"\n}",
				expected: "{\n\"name\": \"John\",\n\"age\": 30,\n\"city\": \"New York\"\n}",
				wantErr:  false,
			},
			{
				name:     "XML structure",
				input:    "<root>\n  <person>\n    <name>John</name>\n    <age>30</age>\n  </person>\n</root>",
				expected: "<root>\n<person>\n<name>John</name>\n<age>30</age>\n</person>\n</root>",
				wantErr:  false,
			},
			{
				name:     "indented HTML",
				input:    "<div>\n  <p>Hello world</p>\n  <ul>\n    <li>Item 1</li>\n  </ul>\n</div>",
				expected: "<div>\n<p>Hello world</p>\n<ul>\n<li>Item 1</li>\n</ul>\n</div>",
				wantErr:  false,
			},
			{
				name:     "multiline with YAML",
				input:    "---\nname: John\nage: 30\nhobbies:\n  - reading\n  - running\n---",
				expected: "---\nname: John\nage: 30\nhobbies:\n- reading\n- running\n---",
				wantErr:  false,
			},
			{
				name:     "code with trailing whitespace on each line",
				input:    "function hello() {   \n  console.log('world');   \n}   ",
				expected: "function hello() {\nconsole.log('world');\n}",
				wantErr:  false,
			},
			{
				name:     "SQL query with comments",
				input:    "SELECT *  -- Get all columns\nFROM users  -- From users table\nWHERE age > 18;  -- Adults only",
				expected: "SELECT * -- Get all columns\nFROM users -- From users table\nWHERE age > 18; -- Adults only",
				wantErr:  false,
			},
			{
				name:     "markdown with fenced code and language",
				input:    "```go\nfunc main() {\n\tfmt.Println(\"Hello, world!\")\n}\n```",
				expected: "```go\nfunc main() {\nfmt.Println(\"Hello, world!\")\n}\n```",
				wantErr:  false,
			},
			{
				name:     "text with emoji list items",
				input:    "Shopping List:\n\n🍎 Apples\n🍌 Bananas\n🥕 Carrots",
				expected: "Shopping List:\n\n🍎 Apples\n🍌 Bananas\n🥕 Carrots",
				wantErr:  false,
			},
		},
		"Real-world Examples": {
			{
				name:     "text with URLs",
				input:    "Visit https://example.com/test?param=value",
				expected: "Visit https://example.com/test?param=value",
				wantErr:  false,
			},
			{
				name:     "text with email addresses",
				input:    "Contact user@example.com   for support",
				expected: "Contact user@example.com for support",
				wantErr:  false,
			},
			{
				name:     "text with special characters",
				input:    "Hello & world! This costs $9.99 + tax",
				expected: "Hello & world! This costs $9.99 + tax",
				wantErr:  false,
			},
			{
				name:     "text with code snippets and variables",
				input:    "Use the `x = y + z` formula where `y` is width and `z` is height.",
				expected: "Use the `x = y + z` formula where `y` is width and `z` is height.",
				wantErr:  false,
			},
			{
				name:     "text with nested parentheses",
				input:    "This function (including the inner (nested) part) works well.",
				expected: "This function (including the inner (nested) part) works well.",
				wantErr:  false,
			},
			{
				name:     "text with bullet points and dashes",
				input:    "Key points:\n- First point\n- Second point\n  - Sub-point\n- Third point",
				expected: "Key points:\n- First point\n- Second point\n- Sub-point\n- Third point",
				wantErr:  false,
			},
			{
				name:     "text with numbered list",
				input:    "Steps:\n1. First step\n2. Second step\n   a. Sub-step\n3. Third step",
				expected: "Steps:\n1. First step\n2. Second step\na. Sub-step\n3. Third step",
				wantErr:  false,
			},
			{
				name:     "chat message with emoji reactions",
				input:    "Great job! 👍 I really like this approach 🎉",
				expected: "Great job! 👍 I really like this approach 🎉",
				wantErr:  false,
			},
			{
				name:     "technical error message",
				input:    "Error: Cannot connect to database (code: DB_CONN_REFUSED)\nDetails: Connection timed out after 30s",
				expected: "Error: Cannot connect to database (code: DB_CONN_REFUSED)\nDetails: Connection timed out after 30s",
				wantErr:  false,
			},
			{
				name:     "text with SQL query",
				input:    "Try this query: SELECT * FROM users WHERE age > 18 ORDER BY name;",
				expected: "Try this query: SELECT * FROM users WHERE age > 18 ORDER BY name;",
				wantErr:  false,
			},
			{
				name:     "text with math formulas",
				input:    "The formula is: y = mx + b where m is the slope.",
				expected: "The formula is: y = mx + b where m is the slope.",
				wantErr:  false,
			},
			{
				name:     "text with timestamps",
				input:    "The event starts at 2023-05-15T14:30:00Z and ends at 2023-05-15T16:30:00Z.",
				expected: "The event starts at 2023-05-15T14:30:00Z and ends at 2023-05-15T16:30:00Z.",
				wantErr:  false,
			},
			{
				name:     "text with file paths",
				input:    "Save the file to C:\\Users\\John\\Documents\\report.pdf or /home/user/documents/report.pdf",
				expected: "Save the file to C:\\Users\\John\\Documents\\report.pdf or /home/user/documents/report.pdf",
				wantErr:  false,
			},
			{
				name:     "text with JSON example",
				input:    "Send this JSON: {\"name\":\"John\", \"age\":30, \"city\":\"New York\"}",
				expected: "Send this JSON: {\"name\":\"John\", \"age\":30, \"city\":\"New York\"}",
				wantErr:  false,
			},
			{
				name:     "text with camelCase and snake_case identifiers",
				input:    "Use getUserProfile() instead of get_user_profile() for consistency.",
				expected: "Use getUserProfile() instead of get_user_profile() for consistency.",
				wantErr:  false,
			},
			{
				name:     "text with version numbers",
				input:    "Upgrade to version 2.5.3-beta.1 from 2.4.9 for the latest features.",
				expected: "Upgrade to version 2.5.3-beta.1 from 2.4.9 for the latest features.",
				wantErr:  false,
			},
			{
				name:     "text with IP addresses and ports",
				input:    "Connect to 192.168.1.100:8080 or [2001:db8::1]:8080 for the service.",
				expected: "Connect to 192.168.1.100:8080 or [2001:db8::1]:8080 for the service.",
				wantErr:  false,
			},
			{
				name:     "text with hashtags and mentions",
				input:    "Check out #golang updates from @golang_news latest post.",
				expected: "Check out #golang updates from @golang_news latest post.",
				wantErr:  false,
			},
			{
				name:     "text with quotes and apostrophes",
				input:    "She said, \"That's not what I meant.\" We weren't sure what to do.",
				expected: "She said, \"That's not what I meant.\" We weren't sure what to do.",
				wantErr:  false,
			},
			{
				name:     "text with command-line arguments",
				input:    "Run `script.sh --verbose --output=/tmp/log.txt` to execute with logging.",
				expected: "Run `script.sh --verbose --output=/tmp/log.txt` to execute with logging.",
				wantErr:  false,
			},
		},
		"Emoji Handling": {
			{
				name:     "basic emoji",
				input:    "Hello 😊 world",
				expected: "Hello 😊 world",
				wantErr:  false,
			},
			{
				name:     "multiple emojis",
				input:    "Hello 😊 👋 🌎 world",
				expected: "Hello 😊 👋 🌎 world",
				wantErr:  false,
			},
			{
				name:     "emojis with whitespace",
				input:    "Hello   😊   world",
				expected: "Hello 😊 world",
				wantErr:  false,
			},
			{
				name:     "emojis at start and end",
				input:    "😊 Hello world 👋",
				expected: "😊 Hello world 👋",
				wantErr:  false,
			},
			{
				name:     "only emoji",
				input:    "😊",
				expected: "😊",
				wantErr:  false,
			},
			{
				name:     "emoji with newlines",
				input:    "Hello\n😊\nworld",
				expected: "Hello\n😊\nworld",
				wantErr:  false,
			},
			{
				name:     "complex emoji (ZWJ sequence)",
				input:    "Family: 👨‍👩‍👧‍👦",
				expected: "Family: 👨‍👩‍👧‍👦",
				wantErr:  false,
			},
			{
				name:     "skin tone modifier",
				input:    "Hi 👋🏽 world",
				expected: "Hi 👋🏽 world",
				wantErr:  false,
			},
			{
				name:     "flag emoji",
				input:    "Countries: 🇧🇷 🇯🇵 🇺🇸 🇪🇺",
				expected: "Countries: 🇧🇷 🇯🇵 🇺🇸 🇪🇺",
				wantErr:  false,
			},
			{
				name:     "multiple complex emojis",
				input:    "👩‍💻 and 👨‍👩‍👧 with 🏳️‍🌈",
				expected: "👩‍💻 and 👨‍👩‍👧 with 🏳️‍🌈",
				wantErr:  false,
			},
			{
				name:     "emoji with variation selector",
				input:    "Heart: ❤️ vs text heart: ❤︎",
				expected: "Heart: ❤️ vs text heart: ❤︎",
				wantErr:  false,
			},
			{
				name:     "emoji in metadata",
				input:    "[2025-03-06T22:30:11+01:00] UID 123456 (😊): Message with emoji",
				expected: "Message with emoji",
				wantErr:  false,
			},
			{
				name:     "emoji with text combining diacritical marks",
				input:    "a\u0300bc 😊 xyz\u0301",
				expected: "a\u0300bc 😊 xyz\u0301",
				wantErr:  false,
			},
			{
				name:     "emoji with keycap sequence",
				input:    "Press 5️⃣ to continue",
				expected: "Press 5️⃣ to continue",
				wantErr:  false,
			},
			{
				name:     "regional indicator symbols",
				input:    "Letters: 🇦 🇧 🇨 vs Flags: 🇬🇧 🇩🇪",
				expected: "Letters: 🇦 🇧 🇨 vs Flags: 🇬🇧 🇩🇪",
				wantErr:  false,
			},
			{
				name:     "emoji with presentation selector",
				input:    "Watch: ⌚️ vs text watch: ⌚",
				expected: "Watch: ⌚️ vs text watch: ⌚",
				wantErr:  false,
			},
			{
				name:     "emojis with invisible joins",
				input:    "Welcome to 🏳️‍⚧️ community and the 👨🏾‍🦱 club",
				expected: "Welcome to 🏳️‍⚧️ community and the 👨🏾‍🦱 club",
				wantErr:  false,
			},
			{
				name:     "recent complex emojis",
				input:    "New emojis: 🧠 🦾 🫶 🫡",
				expected: "New emojis: 🧠 🦾 🫶 🫡",
				wantErr:  false,
			},
			{
				name:     "emoji with metadata and extra whitespace",
				input:    "[2025-03-06T22:30:11Z] UID 123 (@user):    Hello   🌍  !  ",
				expected: "Hello 🌍 !",
				wantErr:  false,
			},
			{
				name:     "text with emoji sequences",
				input:    "Reactions: 👨‍👩‍👧‍👦👨‍👨‍👧‍👦👩‍👩‍👧‍👦",
				expected: "Reactions: 👨‍👩‍👧‍👦👨‍👨‍👧‍👦👩‍👩‍👧‍👦",
				wantErr:  false,
			},
			{
				name:     "all emoji paragraph",
				input:    "🌍🌎🌏🌐🌑🌒🌓🌔🌕🌖🌗🌘🌙🌚🌛🌜🌝",
				expected: "🌍🌎🌏🌐🌑🌒🌓🌔🌕🌖🌗🌘🌙🌚🌛🌜🌝",
				wantErr:  false,
			},
		},
	}

	// Run all test groups as subtests
	for groupName, testCases := range testGroups {
		// Capture range variable
		groupName := groupName

		t.Run(groupName, func(t *testing.T) {
			t.Parallel()

			for _, tc := range testCases {
				// Capture range variable
				tc := tc

				t.Run(tc.name, func(t *testing.T) {
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
		})
	}
}
