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
		{"Basic Input", "text with uppercase letters", "HELLO WORLD", "HELLO WORLD", false},
		{"Basic Input", "text with mixed case", "HeLLo WoRLd", "HeLLo WoRLd", false},
		{"Basic Input", "text with numbers", "Hello 123 World", "Hello 123 World", false},
		{"Basic Input", "text with special characters", "Hello, World! #$%^&*()", "Hello, World! #$%^&*()", false},
		{"Basic Input", "text with unicode characters", "Hello 你好 World", "Hello 你好 World", false},
		{"Basic Input", "zero-width space", "\u200B", "", true},
		{"Basic Input", "null character", "\u0000", "", true},
		{"Basic Input", "text with control characters", "Hello\u0001World", "HelloWorld", false},
		{"Basic Input", "very long text", strings.Repeat("Lorem ipsum dolor sit amet. ", 100), strings.Repeat("Lorem ipsum dolor sit amet. ", 100), false},
		{"Basic Input", "only numbers", "12345", "12345", false},

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
		{"Metadata", "Different timestamp format", "[06-03-2025 22:30:11] UID 123456: Not a valid metadata format", "[06-03-2025 22:30:11] UID 123456: Not a valid metadata format", false},
		{"Metadata", "With incomplete timestamp", "[2025-03-06T22:30] UID 123456: Invalid metadata", "[2025-03-06T22:30] UID 123456: Invalid metadata", false},
		{"Metadata", "With invalid UID format", "[2025-03-06T22:30:11+01:00] User-123: Message content", "[2025-03-06T22:30:11+01:00] User-123: Message content", false},
		{"Metadata", "Metadata with different brackets", "(2025-03-06T22:30:11+01:00) UID 123456: Not valid metadata", "(2025-03-06T22:30:11+01:00) UID 123456: Not valid metadata", false},
		{"Metadata", "With timezone -08:00", "[2025-03-06T14:30:11-08:00] UID 123456: Message with negative timezone", "Message with negative timezone", false},
		{"Metadata", "With unusual but valid timezone +13:45", "[2025-03-06T22:30:11+13:45] UID 123456: Message with unusual timezone", "Message with unusual timezone", false},
		{"Metadata", "With milliseconds and microseconds", "[2025-03-06T22:30:11.123456+01:00] UID 123456: With microseconds", "With microseconds", false},
		{"Metadata", "With metadata and empty message", "[2025-03-06T22:30:11+01:00] UID 123456 (@username):   ", "", true},
		{"Metadata", "With number-only username", "[2025-03-06T22:30:11+01:00] UID 123456 (12345): Number username", "Number username", false},
		{"Metadata", "With very long username", "[2025-03-06T22:30:11+01:00] UID 123456 (" + strings.Repeat("a", 100) + "): Long username", "Long username", false},

		// Whitespace Handling
		{"Whitespace", "text with multiple spaces", "hello   world", "hello world", false},
		{"Whitespace", "text with leading and trailing spaces", "  hello world  ", "hello world", false},
		{"Whitespace", "text with tabs", "hello\tworld\ttest", "hello world test", false},
		{"Whitespace", "text with mixed whitespace", "hello \t \r\n world", "hello\nworld", false},
		{"Whitespace", "text with consecutive spaces between words", "hello      world      test", "hello world test", false},
		{"Whitespace", "text with spaces after punctuation", "hello,   world!   ", "hello, world!", false},
		{"Whitespace", "text with spaces around brackets", "hello   (   world   )   !", "hello ( world ) !", false},
		{"Whitespace", "text with many consecutive spaces", "hello" + strings.Repeat(" ", 100) + "world", "hello world", false},
		{"Whitespace", "text with non-breaking spaces", "hello\u00A0\u00A0world", "hello world", false},
		{"Whitespace", "text with various unicode spaces", "hello\u2000\u2001\u2002\u2003world", "hello world", false},
		{"Whitespace", "text with zero-width spaces", "hello\u200Bworld", "helloworld", false},
		{"Whitespace", "text with vertical tab", "hello\vworld", "hello world", false},
		{"Whitespace", "text with form feed", "hello\fworld", "hello world", false},
		{"Whitespace", "text with only various whitespaces", "\u2000\u2001\u2002\u00A0 \t\n\r\f\v", "", true},
		{"Whitespace", "text with leading tabs", "\t\t\thello world", "hello world", false},
		{"Whitespace", "text with trailing tabs", "hello world\t\t\t", "hello world", false},
		{"Whitespace", "text with whitespace between every character", "h e l l o w o r l d", "h e l l o w o r l d", false},
		{"Whitespace", "text with whitespace pattern", "  hello  world  test  example  ", "hello world test example", false},
		{"Whitespace", "text with ideographic space", "hello\u3000world", "hello world", false},
		{"Whitespace", "text with en quad space", "hello\u2000world", "hello world", false},

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
		{"Newlines", "text with paragraphs and whitespace", "  P1  \n\n  P2  \n\n  P3  ", "P1\n\nP2\n\nP3", false},
		{"Newlines", "alternate line pattern", "Line1\nLine2\n\nLine3\nLine4\n\nLine5", "Line1\nLine2\n\nLine3\nLine4\n\nLine5", false},
		{"Newlines", "text with five consecutive newlines", "Before\n\n\n\n\nAfter", "Before\n\nAfter", false},
		{"Newlines", "text with ten consecutive newlines", "Before" + strings.Repeat("\n", 10) + "After", "Before\n\nAfter", false},
		{"Newlines", "text with mixed whitespace and newlines", "Line1 \t \nLine2 \t \n \t \nLine3", "Line1\nLine2\n\nLine3", false},
		{"Newlines", "list with newlines", "- Item 1\n- Item 2\n- Item 3", "- Item 1\n- Item 2\n- Item 3", false},
		{"Newlines", "text with newlines inside words", "hel\nlo wor\nld", "hel\nlo wor\nld", false},
		{"Newlines", "text with spaces before newlines", "Line1   \nLine2   \nLine3", "Line1\nLine2\nLine3", false},
		{"Newlines", "text with spaces after newlines", "Line1\n   Line2\n   Line3", "Line1\nLine2\nLine3", false},
		{"Newlines", "only whitespace and newlines", "  \n  \n  \n  ", "", true},

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
		{"Line Endings", "CRLF with trailing whitespace", "Line 1\r\n  \r\nLine 2", "Line 1\n\nLine 2", false},
		{"Line Endings", "CR with trailing whitespace", "Line 1\r  \rLine 2", "Line 1\n\nLine 2", false},
		{"Line Endings", "Mixed line endings with indentation", "  Line 1\r\n  Line 2\r  Line 3\n  Line 4", "Line 1\nLine 2\nLine 3\nLine 4", false},
		{"Line Endings", "Complex mixed line ending document", "Para 1\r\nLine 2\r\n\r\nPara 2\rLine 2\r\rPara 3\nLine 2\n\nPara 4", "Para 1\nLine 2\n\nPara 2\nLine 2\n\nPara 3\nLine 2\n\nPara 4", false},
		{"Line Endings", "Non-standard line ending combinations", "Line 1\r\n\rLine 2\n\r\nLine 3", "Line 1\n\nLine 2\n\nLine 3", false},
		{"Line Endings", "Line endings in code blocks", "```\r\ncode line 1\r\ncode line 2\r\n```", "```\ncode line 1\ncode line 2\n```", false},
		{"Line Endings", "Line endings with UTF-8 BOM", "\uFEFFLine 1\r\nLine 2", "Line 1\nLine 2", false},
		{"Line Endings", "Mixed line endings with zero-width spaces", "Line 1\u200B\r\nLine 2\u200B\rLine 3", "Line 1\nLine 2\nLine 3", false},
		{"Line Endings", "Nested line ending patterns", "Level 1\r\n  Level 2\r\n    Level 3\r\n  Level 2\r\nLevel 1", "Level 1\nLevel 2\nLevel 3\nLevel 2\nLevel 1", false},
		{"Line Endings", "Line endings with trailing CR only", "Line 1\r", "Line 1", false},

		// Multiline Content
		{"Multiline", "text with code snippets", "```javascript\nfunction hello() {\n  console.log('world');\n}\n```", "```javascript\nfunction hello() {\nconsole.log('world');\n}\n```", false},
		{"Multiline", "text with complex formatting", "# Title\n\n## Subtitle\n\n- List item 1\n- List item 2\n\n> Blockquote\n\nParagraph", "# Title\n\n## Subtitle\n\n- List item 1\n- List item 2\n\n> Blockquote\n\nParagraph", false},
		{"Multiline", "markdown with code blocks and indentation", "# Code Example\n\n```python\ndef hello():\n    print('Hello, world!')\n    return True\n```\n\nResult: `True`", "# Code Example\n\n```python\ndef hello():\nprint('Hello, world!')\nreturn True\n```\n\nResult: `True`", false},
		{"Multiline", "JSON structure", "{\n  \"name\": \"John\",\n  \"age\": 30,\n  \"city\": \"New York\"\n}", "{\n\"name\": \"John\",\n\"age\": 30,\n\"city\": \"New York\"\n}", false},
		{"Multiline", "code with trailing whitespace on each line", "function hello() {   \n  console.log('world');   \n}   ", "function hello() {\nconsole.log('world');\n}", false},
		{"Multiline", "nested code blocks", "````\n```\nNested code block\n```\n````", "````\n```\nNested code block\n```\n````", false},
		{"Multiline", "markdown tables", "| Header 1 | Header 2 |\n|----------|----------|\n| Cell 1   | Cell 2   |", "| Header 1 | Header 2 |\n|----------|----------|\n| Cell 1 | Cell 2 |", false},
		{"Multiline", "html structure", "<div>\n  <h1>Title</h1>\n  <p>Paragraph</p>\n</div>", "<div>\n<h1>Title</h1>\n<p>Paragraph</p>\n</div>", false},
		{"Multiline", "multi-language code blocks", "```javascript\nconst x = 1;\n```\n\n```python\nx = 1\n```", "```javascript\nconst x = 1;\n```\n\n```python\nx = 1\n```", false},
		{"Multiline", "list with indented items", "* Item 1\n  * Subitem 1.1\n  * Subitem 1.2\n* Item 2", "* Item 1\n* Subitem 1.1\n* Subitem 1.2\n* Item 2", false},
		{"Multiline", "diff blocks", "```diff\n- removed line\n+ added line\n  unchanged line\n```", "```diff\n- removed line\n+ added line\nunchanged line\n```", false},
		{"Multiline", "complex document structure", "# Main Title\n\n## Section 1\n\nText paragraph.\n\n- List item 1\n- List item 2\n\n### Subsection\n\n> Quote\n\n```code\nfunc main() {}\n```", "# Main Title\n\n## Section 1\n\nText paragraph.\n\n- List item 1\n- List item 2\n\n### Subsection\n\n> Quote\n\n```code\nfunc main() {}\n```", false},
		{"Multiline", "yaml structure", "---\nname: John\nage: 30\nitems:\n  - item1\n  - item2\n---", "---\nname: John\nage: 30\nitems:\n- item1\n- item2\n---", false},
		{"Multiline", "tabbed content with whitespace", "Title\n\tIndented line 1\n\t  Indented line 2\n\t\t\tIndented line 3", "Title\nIndented line 1\nIndented line 2\nIndented line 3", false},
		{"Multiline", "xml structure", "<root>\n  <element attr=\"value\">\n    <child>text</child>\n  </element>\n</root>", "<root>\n<element attr=\"value\">\n<child>text</child>\n</element>\n</root>", false},
		{"Multiline", "multiple paragraphs with varying indentation", "Paragraph 1\nstill paragraph 1\n\n  Paragraph 2\n  still paragraph 2\n\n    Paragraph 3", "Paragraph 1\nstill paragraph 1\n\nParagraph 2\nstill paragraph 2\n\nParagraph 3", false},
		{"Multiline", "fenced code blocks with tildes", "~~~\ncode block\nusing tildes\n~~~", "~~~\ncode block\nusing tildes\n~~~", false},
		{"Multiline", "mixed code and prose", "This is text\n\n```\nThis is code\n```\n\nThis is more text", "This is text\n\n```\nThis is code\n```\n\nThis is more text", false},
		{"Multiline", "multi-paragraph text with quotes", "First paragraph.\n\n> This is a quote\n> spanning multiple lines\n\nLast paragraph.", "First paragraph.\n\n> This is a quote\n> spanning multiple lines\n\nLast paragraph.", false},
		{"Multiline", "deeply nested structure", "- Level 1\n  - Level 2\n    - Level 3\n      - Level 4\n        - Level 5", "- Level 1\n- Level 2\n- Level 3\n- Level 4\n- Level 5", false},

		// Real-world Examples
		{"Real-world", "text with URLs", "Visit https://example.com/test?param=value", "Visit https://example.com/test?param=value", false},
		{"Real-world", "text with email addresses", "Contact user@example.com   for support", "Contact user@example.com for support", false},
		{"Real-world", "text with special characters", "Hello & world! This costs $9.99 + tax", "Hello & world! This costs $9.99 + tax", false},
		{"Real-world", "technical error message", "Error: Cannot connect to database (code: DB_CONN_REFUSED)\nDetails: Connection timed out after 30s", "Error: Cannot connect to database (code: DB_CONN_REFUSED)\nDetails: Connection timed out after 30s", false},
		{"Real-world", "text with SQL query", "Try this query: SELECT * FROM users WHERE age > 18 ORDER BY name;", "Try this query: SELECT * FROM users WHERE age > 18 ORDER BY name;", false},
		{"Real-world", "text with file paths", "Save the file to C:\\Users\\John\\Documents\\report.pdf or /home/user/documents/report.pdf", "Save the file to C:\\Users\\John\\Documents\\report.pdf or /home/user/documents/report.pdf", false},
		{"Real-world", "text with quotes and apostrophes", "She said, \"That's not what I meant.\" We weren't sure what to do.", "She said, \"That's not what I meant.\" We weren't sure what to do.", false},
		{"Real-world", "log message with IP addresses", "Access from 192.168.1.1 to 10.0.0.1 was blocked by firewall rule #5", "Access from 192.168.1.1 to 10.0.0.1 was blocked by firewall rule #5", false},
		{"Real-world", "text with hashtags", "Check out our latest post #NewFeature #ProductUpdate #Technology", "Check out our latest post #NewFeature #ProductUpdate #Technology", false},
		{"Real-world", "command line instructions", "Run `npm install --save-dev webpack` and then `npm start` to begin development", "Run `npm install --save-dev webpack` and then `npm start` to begin development", false},
		{"Real-world", "text with phone numbers", "Call us at +1 (555) 123-4567 or +44 20 1234 5678", "Call us at +1 (555) 123-4567 or +44 20 1234 5678", false},
		{"Real-world", "text with date formats", "Meeting scheduled for 2025-04-15 at 15:30 (or 4/15/25 3:30 PM)", "Meeting scheduled for 2025-04-15 at 15:30 (or 4/15/25 3:30 PM)", false},
		{"Real-world", "text with mathematical expressions", "The formula is x² + 2x + 1 = (x + 1)²", "The formula is x² + 2x + 1 = (x + 1)²", false},
		{"Real-world", "git command output", "$ git status\nOn branch main\nYour branch is up to date with 'origin/main'.\nnothing to commit, working tree clean", "$ git status\nOn branch main\nYour branch is up to date with 'origin/main'.\nnothing to commit, working tree clean", false},
		{"Real-world", "text with HTML entities", "Copyright &copy; 2025 &amp; all rights reserved", "Copyright &copy; 2025 &amp; all rights reserved", false},
		{"Real-world", "programming language identifiers", "Use MyClass.staticMethod() or instanceObj.method() to call methods", "Use MyClass.staticMethod() or instanceObj.method() to call methods", false},
		{"Real-world", "JSON API response", "{\n  \"status\": \"success\",\n  \"data\": {\n    \"id\": 123,\n    \"name\": \"Product\"\n  }\n}", "{\n\"status\": \"success\",\n\"data\": {\n\"id\": 123,\n\"name\": \"Product\"\n}\n}", false},
		{"Real-world", "URLs with query parameters", "https://example.com/search?q=test&page=1&sort=desc&filter[]=category1&filter[]=category2", "https://example.com/search?q=test&page=1&sort=desc&filter[]=category1&filter[]=category2", false},
		{"Real-world", "text with version numbers", "Upgrade to version 2.5.3-beta.1 from 2.4.9", "Upgrade to version 2.5.3-beta.1 from 2.4.9", false},
		{"Real-world", "stacktrace", "Exception in thread \"main\" java.lang.NullPointerException\n    at com.example.Main.method(Main.java:25)\n    at com.example.Main.main(Main.java:10)", "Exception in thread \"main\" java.lang.NullPointerException\nat com.example.Main.method(Main.java:25)\nat com.example.Main.main(Main.java:10)", false},

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
		{"Emoji", "emoji sequences", "We all 👩‍👩‍👧‍👧 together", "We all 👩‍👩‍👧‍👧 together", false},
		{"Emoji", "activity emojis", "Sports: ⚽ 🏀 🏈 ⚾ 🎾 🏐 🏉 🎱", "Sports: ⚽ 🏀 🏈 ⚾ 🎾 🏐 🏉 🎱", false},
		{"Emoji", "food emojis", "I love 🍕 🍔 🍟 🌭 🍿", "I love 🍕 🍔 🍟 🌭 🍿", false},
		{"Emoji", "animal emojis", "Farm: 🐶 🐱 🐭 🐹 🐰 🦊 🐻 🐼", "Farm: 🐶 🐱 🐭 🐹 🐰 🦊 🐻 🐼", false},
		{"Emoji", "text with emoji in the middle of words", "wonder🌟ful ex🎉perience", "wonder🌟ful ex🎉perience", false},
		{"Emoji", "emoji with variation selectors", "☺️ vs ☺", "☺️ vs ☺", false},
		{"Emoji", "emoji with multiple skin tones", "👩🏻 👩🏼 👩🏽 👩🏾 👩🏿", "👩🏻 👩🏼 👩🏽 👩🏾 👩🏿", false},
		{"Emoji", "emoji with gender modifiers", "👮‍♀️ 👮‍♂️ 👷‍♀️ 👷‍♂️", "👮‍♀️ 👮‍♂️ 👷‍♀️ 👷‍♂️", false},
		{"Emoji", "numbered emoji", "#️⃣1️⃣2️⃣3️⃣", "#️⃣1️⃣2️⃣3️⃣", false},
		{"Emoji", "directional emoji", "◀️ ▶️ ⬆️ ⬇️", "◀️ ▶️ ⬆️ ⬇️", false},
		{"Emoji", "only multiple emojis", "😀😃😄😁😆", "😀😃😄😁😆", false},
	}

	// Iterate over test cases
	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.group+"/"+tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := service.Sanitize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Sanitize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("Sanitize() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
