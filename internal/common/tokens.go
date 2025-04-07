package common

import (
	"log/slog"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

var (
	tokenizer     *tiktoken.Tiktoken
	tokenizerOnce sync.Once
	tokenizerErr  error
)

// getTokenizer returns a singleton instance of the tiktoken tokenizer
func getTokenizer() (*tiktoken.Tiktoken, error) {
	tokenizerOnce.Do(func() {
		tokenizer, tokenizerErr = tiktoken.GetEncoding("cl100k_base")
		if tokenizerErr != nil {
			slog.Error("failed to initialize tokenizer", "error", tokenizerErr)
		}
	})
	return tokenizer, tokenizerErr
}

// CountTokens returns the number of tokens in text
func CountTokens(text string) (int, error) {
	tk, err := getTokenizer()
	if err != nil {
		return 0, err
	}

	return len(tk.Encode(text, nil, nil)), nil
}

// FilterContent filters a slice of strings to fit within maxTokens
func FilterContent(items []string, maxTokens int, newest bool) ([]string, error) {
	if len(items) == 0 || maxTokens <= 0 {
		return nil, nil
	}

	tk, err := getTokenizer()
	if err != nil {
		return nil, err
	}

	filtered := make([]string, 0, len(items))
	available := maxTokens
	start := 0
	end := len(items)
	step := 1

	if newest {
		start = len(items) - 1
		end = -1
		step = -1
	}

	for i := start; i != end; i += step {
		tokens := len(tk.Encode(items[i], nil, nil))
		if tokens > available {
			continue
		}

		available -= tokens
		if newest {
			filtered = append([]string{items[i]}, filtered...)
		} else {
			filtered = append(filtered, items[i])
		}
	}

	return filtered, nil
}
