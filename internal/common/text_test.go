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
		// ==========================================
		// 1. EMPTY AND WHITESPACE-ONLY INPUTS
		// ==========================================
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single character",
			input:    "a",
			expected: "a",
		},
		{
			name:     "single space",
			input:    " ",
			expected: "",
		},
		{
			name:     "only spaces",
			input:    "   ",
			expected: "",
		},
		{
			name:     "single tab",
			input:    "\t",
			expected: "",
		},
		{
			name:     "only tabs",
			input:    "\t\t\t",
			expected: "",
		},
		{
			name:     "single newline",
			input:    "\n",
			expected: "",
		},
		{
			name:     "only newlines",
			input:    "\n\n\n\n",
			expected: "",
		},
		{
			name:     "single carriage return",
			input:    "\r",
			expected: "",
		},
		{
			name:     "mixed whitespace characters",
			input:    " \t \n \t\n\t ",
			expected: "",
		},

		// ==========================================
		// 2. UNICODE WHITESPACE HANDLING
		// ==========================================
		// 2.1 Basic Unicode Spaces
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
			name:     "hair space",
			input:    "hello\u200Aworld", // hair space
			expected: "hello world",
		},
		{
			name:     "ideographic space",
			input:    "hello\u3000world", // ideographic space
			expected: "hello world",
		},

		// 2.2 Additional Unicode Spaces
		{
			name:     "zero width space",
			input:    "hello\u200Bworld", // zero width space
			expected: "hello\u200Bworld", // zero width space is preserved
		},
		{
			name:     "narrow no-break space",
			input:    "hello\u202Fworld", // narrow no-break space
			expected: "hello world",
		},
		{
			name:     "medium mathematical space",
			input:    "hello\u205Fworld", // medium mathematical space
			expected: "hello world",
		},
		{
			name:     "ogham space mark",
			input:    "hello\u1680world", // ogham space mark
			expected: "hello world",
		},
		{
			name:     "mongolian vowel separator",
			input:    "hello\u180Eworld", // mongolian vowel separator
			expected: "hello\u180Eworld", // mongolian vowel separator is preserved
		},
		{
			name:     "figure space",
			input:    "hello\u2007world", // figure space
			expected: "hello world",
		},
		{
			name:     "punctuation space",
			input:    "hello\u2008world", // punctuation space
			expected: "hello world",
		},
		{
			name:     "six-per-em space",
			input:    "hello\u2006world", // six-per-em space
			expected: "hello world",
		},

		// ==========================================
		// 3. SINGLE LINE TEXT NORMALIZATION
		// ==========================================
		// 3.1 Basic Single Line Cases
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
		{
			name:     "alternating spaces and tabs",
			input:    " \t \t \t hello \t \t \t world \t \t \t",
			expected: "hello world",
		},

		// 3.2 Punctuation and Special Characters
		{
			name:     "spaces around punctuation",
			input:    "Hello  ,  world  !  How  are  you  ?",
			expected: "Hello , world ! How are you ?",
		},
		{
			name:     "spaces in numbers",
			input:    "The price is  $  1  000  000",
			expected: "The price is $ 1 000 000",
		},
		{
			name:     "spaces in dates",
			input:    "Date  :  2023  -  04  -  01",
			expected: "Date : 2023 - 04 - 01",
		},
		{
			name:     "spaces in time",
			input:    "Time  :  12  :  30  :  45",
			expected: "Time : 12 : 30 : 45",
		},
		{
			name:     "spaces in email",
			input:    "Email  :  user  @  example  .  com",
			expected: "Email : user @ example . com",
		},
		{
			name:     "spaces in URL",
			input:    "URL  :  https  :  //  example  .  com  /  path",
			expected: "URL : https : // example . com / path",
		},

		// ==========================================
		// 4. MULTI-LINE TEXT NORMALIZATION
		// ==========================================
		// 4.1 Basic Multi-line Cases
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
			name:     "text with trailing newlines",
			input:    "Hello world\n\n\n\n",
			expected: "Hello world",
		},
		{
			name:     "text with leading newlines",
			input:    "\n\n\n\nHello world",
			expected: "Hello world",
		},

		// 4.2 Paragraphs and Formatting
		{
			name:     "mixed content with empty lines",
			input:    "Title\n\nParagraph 1\n\n\n\nParagraph 2\n\n\n\n\n\nParagraph 3",
			expected: "Title\n\nParagraph 1\n\nParagraph 2\n\nParagraph 3",
		},
		{
			name:     "three paragraphs with mixed spacing",
			input:    "  Paragraph 1  \n\n  Paragraph 2  \n\n  Paragraph 3  ",
			expected: "Paragraph 1\n\nParagraph 2\n\nParagraph 3",
		},
		{
			name:     "indented paragraphs",
			input:    "    Paragraph 1    \n\n        Paragraph 2        \n\n            Paragraph 3            ",
			expected: "Paragraph 1\n\nParagraph 2\n\nParagraph 3",
		},
		{
			name:     "bullet points with mixed spacing",
			input:    "  • Item 1  \n  • Item 2  \n  • Item 3  ",
			expected: "• Item 1\n• Item 2\n• Item 3",
		},
		{
			name:     "numbered list with mixed spacing",
			input:    "  1.  Item 1  \n  2.  Item 2  \n  3.  Item 3  ",
			expected: "1. Item 1\n2. Item 2\n3. Item 3",
		},
		{
			name:     "code block with mixed spacing",
			input:    "  ```  \n  code line 1  \n  code line 2  \n  ```  ",
			expected: "```\ncode line 1\ncode line 2\n```",
		},
		{
			name:     "blockquote with mixed spacing",
			input:    "  >  Quote line 1  \n  >  Quote line 2  \n  >  Quote line 3  ",
			expected: "> Quote line 1\n> Quote line 2\n> Quote line 3",
		},
		{
			name:     "table with mixed spacing",
			input:    "  |  Header 1  |  Header 2  |  \n  |  ---  |  ---  |  \n  |  Cell 1  |  Cell 2  |  ",
			expected: "| Header 1 | Header 2 |\n| --- | --- |\n| Cell 1 | Cell 2 |",
		},

		// ==========================================
		// 5. NEWLINE VARIANT HANDLING
		// ==========================================
		// 5.1 Basic Newline Variants
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

		// 5.2 Complex Newline Variants
		{
			name:     "complex mixed line endings",
			input:    "line1\r\n\r\nline2\r\r\nline3\n\r\n\rline4",
			expected: "line1\n\nline2\n\nline3\n\nline4",
		},
		{
			name:     "mixed CRLF and LF with empty lines",
			input:    "line1\r\n\r\nline2\n\nline3",
			expected: "line1\n\nline2\n\nline3",
		},
		{
			name:     "mixed CR and CRLF with empty lines",
			input:    "line1\r\r\nline2\r\n\rline3",
			expected: "line1\n\nline2\n\nline3",
		},
		{
			name:     "mixed CR, CRLF, and LF",
			input:    "line1\rline2\r\nline3\nline4\r\nline5\rline6\n",
			expected: "line1\nline2\nline3\nline4\nline5\nline6",
		},
		{
			name:     "consecutive different newlines",
			input:    "line1\r\r\n\n\r\nline2",
			expected: "line1\n\nline2",
		},
		{
			name:     "alternating CR and LF",
			input:    "line1\r\nline2\r\nline3\r\nline4\r\nline5",
			expected: "line1\nline2\nline3\nline4\nline5",
		},

		// ==========================================
		// 6. WHITESPACE-ONLY LINES
		// ==========================================
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
		{
			name:     "mixed whitespace lines between paragraphs",
			input:    "Paragraph 1\n \n\t\n   \nParagraph 2\n\t \n  \t\n\t \t\nParagraph 3",
			expected: "Paragraph 1\n\nParagraph 2\n\nParagraph 3",
		},
		{
			name:     "whitespace lines with different characters",
			input:    "Line1\n \nLine2\n\t\nLine3\n \t \nLine4\n\u00A0\nLine5",
			expected: "Line1\n\nLine2\n\nLine3\n\nLine4\n\nLine5",
		},
		{
			name:     "whitespace lines with unicode spaces",
			input:    "Line1\n\u2003\nLine2\n\u2002\nLine3\n\u3000\nLine4",
			expected: "Line1\n\nLine2\n\nLine3\n\nLine4",
		},
		{
			name:     "leading whitespace lines",
			input:    " \n\t\n  \n\t \nLine1\nLine2",
			expected: "Line1\nLine2",
		},
		{
			name:     "trailing whitespace lines",
			input:    "Line1\nLine2\n \n\t\n  \n\t \n",
			expected: "Line1\nLine2",
		},

		// ==========================================
		// 7. LONG WHITESPACE SEQUENCES
		// ==========================================
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
		{
			name:     "long sequence of tabs",
			input:    "Start" + strings.Repeat("\t", 500) + "End",
			expected: "Start End",
		},
		{
			name:     "long sequence of non-breaking spaces",
			input:    "Start" + strings.Repeat("\u00A0", 300) + "End",
			expected: "Start End",
		},
		{
			name:     "long sequence of ideographic spaces",
			input:    "Start" + strings.Repeat("\u3000", 200) + "End",
			expected: "Start End",
		},
		{
			name:     "long sequence of mixed unicode spaces",
			input:    "Start" + strings.Repeat("\u2002\u2003\u2009\u3000", 100) + "End",
			expected: "Start End",
		},
		{
			name:     "long sequence of newlines",
			input:    "Start" + strings.Repeat("\n", 500) + "End",
			expected: "Start\n\nEnd",
		},

		// ==========================================
		// 8. EMOJI HANDLING
		// ==========================================
		// 8.1 Basic Emoji Cases
		{
			name:     "text with simple emojis",
			input:    "Hello  😀  world  👋  !",
			expected: "Hello 😀 world 👋 !",
		},
		{
			name:     "text with emoji at start",
			input:    "😀  Hello  world",
			expected: "😀 Hello world",
		},
		{
			name:     "text with emoji at end",
			input:    "Hello  world  😀",
			expected: "Hello world 😀",
		},
		{
			name:     "text with only emojis",
			input:    "😀  😁  😂  🤣  😃",
			expected: "😀 😁 😂 🤣 😃",
		},

		// 8.2 Complex Emoji Cases
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
			name:     "text with emoji sequences",
			input:    "Emotions:  😀😁😂  🤣😃😄  😅😆😉",
			expected: "Emotions: 😀😁😂 🤣😃😄 😅😆😉",
		},
		{
			name:     "text with keycap emojis",
			input:    "Numbers:  1️⃣  2️⃣  3️⃣  4️⃣  5️⃣",
			expected: "Numbers: 1️⃣ 2️⃣ 3️⃣ 4️⃣ 5️⃣",
		},
		{
			name:     "text with regional indicator symbols",
			input:    "Flags:  🇺🇸  🇯🇵  🇪🇺  🇬🇧  🇨🇦",
			expected: "Flags: 🇺🇸 🇯🇵 🇪🇺 🇬🇧 🇨🇦",
		},
		{
			name:     "text with flag emojis",
			input:    "More flags:  🏁  🏴  🏳️  🏳️‍🌈  🏴‍☠️",
			expected: "More flags: 🏁 🏴 🏳️ 🏳️‍🌈 🏴‍☠️",
		},

		// ==========================================
		// 9. INTERNATIONALIZATION
		// ==========================================
		// 9.1 Latin-based Scripts
		{
			name:     "text with accented characters",
			input:    "  résumé  \n  café  \n  naïve  ",
			expected: "résumé\ncafé\nnaïve",
		},

		// 9.2 Non-Latin Scripts
		{
			name:     "text with non-latin characters",
			input:    "  こんにちは  \n  世界  \n  你好  \n  세계  ",
			expected: "こんにちは\n世界\n你好\n세계",
		},
		{
			name:     "text with Greek characters",
			input:    "  Γειά σου  Κόσμε  ",
			expected: "Γειά σου Κόσμε",
		},
		{
			name:     "text with Cyrillic characters",
			input:    "  Привет  мир  ",
			expected: "Привет мир",
		},
		{
			name:     "text with Thai characters",
			input:    "  สวัสดี  ชาวโลก  ",
			expected: "สวัสดี ชาวโลก",
		},
		{
			name:     "text with Armenian characters",
			input:    "  Բարեւ  աշխարհ  ",
			expected: "Բարեւ աշխարհ",
		},
		{
			name:     "text with Georgian characters",
			input:    "  გამარჯობა  მსოფლიო  ",
			expected: "გამარჯობა მსოფლიო",
		},
		{
			name:     "text with Devanagari characters",
			input:    "  नमस्ते  दुनिया  ",
			expected: "नमस्ते दुनिया",
		},
		{
			name:     "text with Bengali characters",
			input:    "  ওহে  বিশ্ব  ",
			expected: "ওহে বিশ্ব",
		},
		{
			name:     "right-to-left text",
			input:    "  مرحبا  بالعالم  \n  שלום  עולם  ",
			expected: "مرحبا بالعالم\nשלום עולם",
		},

		// 9.3 Mixed Scripts
		{
			name:     "mixed language text",
			input:    "  Hello  世界  !  こんにちは  world  !  ",
			expected: "Hello 世界 ! こんにちは world !",
		},
		{
			name:     "mixed languages, whitespace, and newlines",
			input:    "  English  \n  日本語  \n  Español  \n  Русский  ",
			expected: "English\n日本語\nEspañol\nРусский",
		},

		// ==========================================
		// 10. CONTROL CHARACTER HANDLING
		// ==========================================
		{
			name:     "text with bell character",
			input:    "Hello\u0007World",
			expected: "HelloWorld",
		},
		{
			name:     "text with backspace character",
			input:    "Hello\u0008World",
			expected: "HelloWorld",
		},
		{
			name:     "text with form feed character",
			input:    "Hello\u000CWorld",
			expected: "HelloWorld",
		},
		{
			name:     "text with vertical tab character",
			input:    "Hello\u000BWorld",
			expected: "HelloWorld",
		},
		{
			name:     "text with escape character",
			input:    "Hello\u001BWorld",
			expected: "HelloWorld",
		},
		{
			name:     "text with delete character",
			input:    "Hello\u007FWorld",
			expected: "HelloWorld",
		},
		{
			name:     "text with multiple control characters",
			input:    "Hello\u0007\u0008\u000C\u000B\u001B\u007FWorld",
			expected: "HelloWorld",
		},
		{
			name:     "mixed control characters and whitespace",
			input:    "Hello\u0007  \u0008World\u000C  \u000B!",
			expected: "Hello World !", // Note the space before the exclamation mark
		},

		// ==========================================
		// 11. MIXED AND EDGE CASES
		// ==========================================
		// 11.1 Mixed Content Types
		{
			name:     "mixed whitespace, newlines, and emojis",
			input:    "  Hello  😀  \n\n  World  👋  \n\n  !  ",
			expected: "Hello 😀\n\nWorld 👋\n\n!",
		},
		{
			name:     "mixed everything",
			input:    "  Hello  😀  \n\n  こんにちは  \u0007  \n\n  Привет  \t  \r\n\r\n  مرحبا  👋  ",
			expected: "Hello 😀\n\nこんにちは\n\nПривет\n\nمرحبا 👋",
		},

		// 11.2 Edge Cases
		{
			name:     "very long line",
			input:    strings.Repeat("a", 10000),
			expected: strings.Repeat("a", 10000),
		},
		{
			name:     "very long line with spaces",
			input:    strings.Repeat("a ", 5000),
			expected: strings.Repeat("a ", 4999) + "a",
		},
		{
			name:     "very long line with newlines",
			input:    strings.Repeat("a\n", 5000),
			expected: strings.Repeat("a\n", 4999) + "a",
		},
		{
			name:     "alternating characters and spaces",
			input:    strings.Repeat("a ", 1000),
			expected: strings.Repeat("a ", 999) + "a",
		},
		{
			name:     "repeated special characters",
			input:    strings.Repeat("!@#$%^&*() ", 100),
			expected: strings.Repeat("!@#$%^&*() ", 99) + "!@#$%^&*()",
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
