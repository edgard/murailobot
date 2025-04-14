// Package util_test tests the util package
package util_test

import (
	"testing"

	"github.com/edgard/murailobot/internal/common/util"
)

// TestSanitize tests the Sanitize function with various input types.
// The tests are organized into logical categories using subtests.
func TestSanitize(t *testing.T) {
	t.Parallel()

	// Define a common test case structure for all Sanitize tests
	type sanitizeTestCase struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}

	// Group test cases by functionality for better organization
	testGroups := map[string][]sanitizeTestCase{
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
				name:     "Real reply metadata",
				input:    "[2025-03-06T23:19:51+01:00] BOT: coé beloiro, tá bolado porque tá chovendo na polonia? kkkkkkkk vai dormi mermão",
				expected: "coé beloiro, tá bolado porque tá chovendo na polonia? kkkkkkkk vai dormi mermão",
				wantErr:  false,
			},
			{
				name:     "With complex identifier containing special characters",
				input:    "[2025-03-06T22:30:11+01:00] UID 123-456 (@user.name_with-special.chars): Content with complex ID",
				expected: "Content with complex ID",
				wantErr:  false,
			},
			{
				name:     "With high-precision fractional seconds",
				input:    "[2025-03-06T22:30:11.123456789+01:00] USER: Message with high precision time",
				expected: "Message with high precision time",
				wantErr:  false,
			},
			{
				name:     "With uncommon timezone offset",
				input:    "[2025-03-06T22:30:11+05:45] SYSTEM: Message with Nepal timezone",
				expected: "Message with Nepal timezone",
				wantErr:  false,
			},
			{
				name:     "With multiple colons in identifier",
				input:    "[2025-03-06T22:30:11+01:00] System:Log:Info: The message content",
				expected: "The message content",
				wantErr:  false,
			},
			{
				name:     "Malformed timestamp (shouldn't match)",
				input:    "[2025/03/06 22:30:11] USER: This shouldn't match as metadata",
				expected: "[2025/03/06 22:30:11] USER: This shouldn't match as metadata",
				wantErr:  false,
			},
			{
				name:     "Only metadata",
				input:    "[2025-03-06T23:19:51+01:00] BOT:",
				expected: "",
				wantErr:  true,
			},
		},
		"Basic Text": {
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
				name:     "text with special characters",
				input:    "hello & world",
				expected: "hello & world",
				wantErr:  false,
			},
			{
				name:     "text with numeric and punctuation characters",
				input:    "  1234, 5678!  \n  $9.99  ",
				expected: "1234, 5678!\n$9.99",
				wantErr:  false,
			},
			{
				name:     "only whitespace",
				input:    "   \t\n\r\f\v   ",
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
				name:     "text with non-breaking spaces",
				input:    "hello\u00A0\u00A0world",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with unusual spacing",
				input:    "word\u205Fword\u2060word",
				expected: "word wordword",
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
				name:     "text with four newline characters",
				input:    "hello\n\n\n\nworld",
				expected: "hello\n\nworld",
				wantErr:  false,
			},
			{
				name:     "text with very long newline sequence",
				input:    "hello\n\n\n\n\n\n\nworld",
				expected: "hello\n\nworld",
				wantErr:  false,
			},
			{
				name:     "text with multiple newline characters and spaces",
				input:    "hello \n \n world",
				expected: "hello\n\nworld",
				wantErr:  false,
			},
			{
				name:     "text with mixed spaces and newlines",
				input:    "  hello\n  world  ",
				expected: "hello\nworld",
				wantErr:  false,
			},
			{
				name:     "text with a lot of spaces and newlines",
				input:    "  hello   \n\n\n  world  ",
				expected: "hello\n\nworld",
				wantErr:  false,
			},
			{
				name:     "text with multiple spaces at line breaks",
				input:    "hello    \n    world",
				expected: "hello\nworld",
				wantErr:  false,
			},
			{
				name:     "text with special whitespace sequences",
				input:    "hello\n \n \n \nworld",
				expected: "hello\n\nworld",
				wantErr:  false,
			},
			{
				name:     "only newlines",
				input:    "\n\n\n\n",
				expected: "",
				wantErr:  true,
			},
		},
		"Line Separator Handling": {
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
				name:     "text with line separator",
				input:    "hello\u2028world",
				expected: "hello\nworld",
				wantErr:  false,
			},
			{
				name:     "text with paragraph separator",
				input:    "hello\u2029world",
				expected: "hello\n\nworld",
				wantErr:  false,
			},
			{
				name:     "only line separators",
				input:    "\r\n\r\n",
				expected: "",
				wantErr:  true,
			},
		},
		"Unicode Character Handling": {
			{
				name:     "text with unicode characters",
				input:    "你好，世界",
				expected: "你好，世界",
				wantErr:  false,
			},
			{
				name:     "text with emoji",
				input:    "hello 👋 world 🌍",
				expected: "hello 👋 world 🌍",
				wantErr:  false,
			},
			{
				name:     "text with multiple languages",
				input:    "English 日本語 Español Français   العربية",
				expected: "English 日本語 Español Français العربية",
				wantErr:  false,
			},
			{
				name:     "text with fullwidth characters",
				input:    "ｈｅｌｌｏ　ｗｏｒｌｄ",
				expected: "ｈｅｌｌｏ ｗｏｒｌｄ",
				wantErr:  false,
			},
			{
				name:     "text with combining diacritical marks",
				input:    "n\u0303o\u0308",
				expected: "n\u0303o\u0308",
				wantErr:  false,
			},
			{
				name:     "text with mathematical notation",
				input:    "x² + y² = z²   and   α + β = γ",
				expected: "x² + y² = z² and α + β = γ",
				wantErr:  false,
			},
			{
				name:     "text with quotes and apostrophes",
				input:    "'Single quotes' and \"Double quotes\" and \"Curly quotes\"",
				expected: "'Single quotes' and \"Double quotes\" and \"Curly quotes\"",
				wantErr:  false,
			},
			{
				name:     "text with right-to-left and left-to-right mixed",
				input:    "English مع العربية mixed together",
				expected: "English مع العربية mixed together",
				wantErr:  false,
			},
			{
				name:     "text with unusual Unicode blocks",
				input:    "Musical: \u266A \u266B and technical: ⌘ ⌥ ⇧",
				expected: "Musical: \u266A \u266B and technical: ⌘ ⌥ ⇧",
				wantErr:  false,
			},
			{
				name:     "text with homoglyphs",
				input:    "Regular 'a' vs Cyrillic 'а'",
				expected: "Regular 'a' vs Cyrillic 'а'",
				wantErr:  false,
			},
		},
		"Control Character Handling": {
			{
				name:     "text with zero-width spaces",
				input:    "hello\u200Bworld\u200B",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with zero-width non-joiner",
				input:    "hello\u200Cworld",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with bidirectional characters",
				input:    "Hello world test",
				expected: "Hello world test",
				wantErr:  false,
			},
			{
				name:     "text with control characters",
				input:    "hello\u0000\u0001\u0002world",
				expected: "hello world",
				wantErr:  false,
			},
			{
				name:     "text with byte order mark",
				input:    "\uFEFFHello world",
				expected: "Hello world",
				wantErr:  false,
			},
			{
				name:     "text with soft hyphens",
				input:    "super\u00ADcalifragilistic",
				expected: "supercalifragilistic",
				wantErr:  false,
			},
			{
				name:     "text with Unicode joiners",
				input:    "zero\u2060width\u2060joiner",
				expected: "zerowidthjoiner",
				wantErr:  false,
			},
			{
				name:     "only control characters",
				input:    "\u0000\u0001\u0002",
				expected: "",
				wantErr:  true,
			},
		},
		"Mixed Character Handling": {
			{
				name:     "text with mixed whitespace characters",
				input:    "hello\t \r\nworld\f\vtest",
				expected: "hello\nworld test",
				wantErr:  false,
			},
			{
				name:     "text with multiple consecutive whitespace types",
				input:    "hello\r\n\t \f\vworld",
				expected: "hello\nworld",
				wantErr:  false,
			},
		},
		"Edge Cases": {
			{
				name:     "null bytes in middle of text",
				input:    "Hello\u0000world\u0000test",
				expected: "Hello world test",
				wantErr:  false,
			},
			{
				name:     "mixture of control chars and normal text",
				input:    "\u0001H\u0002e\u0003l\u0004l\u0005o\u0006 \u0007w\u0008o\u000Br\u000Cl\u000Ed",
				expected: "H e l l o w o r l d",
				wantErr:  false,
			},
			{
				name:     "repeated problematic patterns",
				input:    "\r\n\r\n\r\nHello\r\n\r\n\r\nWorld\r\n\r\n\r\n",
				expected: "Hello\n\nWorld",
				wantErr:  false,
			},
			{
				name:     "alternating visible and invisible",
				input:    "A\u200BB\u200BC\u200BD\u200BE\u200BF",
				expected: "A B C D E F",
				wantErr:  false,
			},
		},
		"Specific Content": {
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
				name:     "text with HTML content",
				input:    "<div>This is <b>HTML</b> content</div>",
				expected: "<div>This is <b>HTML</b> content</div>",
				wantErr:  false,
			},
			{
				name:     "text with script tags",
				input:    "Alert: <script>alert('XSS')</script>",
				expected: "Alert: <script>alert('XSS')</script>",
				wantErr:  false,
			},
			{
				name:     "text with SQL injection pattern",
				input:    "Username: admin'; DROP TABLE users; --",
				expected: "Username: admin'; DROP TABLE users; --",
				wantErr:  false,
			},
			{
				name:     "text with code snippets",
				input:    "```javascript\nfunction hello() {\n  console.log('world');\n}\n```",
				expected: "```javascript\nfunction hello() {\nconsole.log('world');\n}\n```",
				wantErr:  false,
			},
			{
				name:     "text with JSON content",
				input:    "Config: {\"key\": \"value\", \"nested\": {\"array\": [1, 2, 3]}}",
				expected: "Config: {\"key\": \"value\", \"nested\": {\"array\": [1, 2, 3]}}",
				wantErr:  false,
			},
		},
	}

	// Create a very long input string (50KB) for the long text test
	var longString string

	pattern := "abcdefghijklmnopqrstuvwxyz0123456789"
	for range 1280 {
		longString += pattern
	}

	// Add the long text test separately
	testGroups["Edge Cases"] = append(testGroups["Edge Cases"], sanitizeTestCase{
		name:     "very long text",
		input:    longString,
		expected: longString,
		wantErr:  false,
	})

	// Add the high concentration of special characters test
	testGroups["Edge Cases"] = append(testGroups["Edge Cases"], sanitizeTestCase{
		name:     "high concentration of special characters",
		input:    "!@#$%^&*()_+{}|:<>?~`-=[]\\;',./!@#$%^&*()_+{}|:<>?~`-=[]\\;',./",
		expected: "!@#$%^&*()_+{}|:<>?~`-=[]\\;',./!@#$%^&*()_+{}|:<>?~`-=[]\\;',./",
		wantErr:  false,
	})

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

					actual, err := util.Sanitize(tc.input)
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
