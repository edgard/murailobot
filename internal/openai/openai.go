package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client encapsulates the OpenAI API configuration.
type Client struct {
	Token       string
	Instruction string
	Model       string
	Temperature float32
	TopP        float32
	HTTPClient  *http.Client
}

func NewClient(token, instruction, model string, temperature, topP float32) (*Client, error) {
	if token == "" || instruction == "" {
		return nil, fmt.Errorf("openai: invalid configuration")
	}
	return &Client{
		Token:       token,
		Instruction: instruction,
		Model:       model,
		Temperature: temperature,
		TopP:        topP,
		HTTPClient:  &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// Call sends messages to OpenAI and returns the generated reply.
func (c *Client) Call(ctx context.Context, messages []map[string]string) (string, error) {
	reqBody := map[string]interface{}{
		"model":       c.Model,
		"temperature": c.Temperature,
		"top_p":       c.TopP,
		"messages":    messages,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("openai: failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("openai: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.Token))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai: request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai: non-200 response: %d - %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("openai: failed to read response: %w", err)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("openai: failed to unmarshal response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai: unexpected response: no choices")
	}
	return result.Choices[0].Message.Content, nil
}
