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

// Client encapsulates the configuration for interacting with the OpenAI API.
type Client struct {
	Token       string
	Instruction string
	Model       string
	Temperature float32
	TopP        float32
	URL         string
	HTTPClient  *http.Client
}

// NewClient creates a new OpenAI client instance.
func NewClient(token, instruction, model string, temperature, topP float32, url string) (*Client, error) {
	if token == "" || instruction == "" {
		return nil, fmt.Errorf("invalid OpenAI configuration")
	}
	return &Client{
		Token:       token,
		Instruction: instruction,
		Model:       model,
		Temperature: temperature,
		TopP:        topP,
		URL:         url,
		HTTPClient:  &http.Client{Timeout: 120 * time.Second},
	}, nil
}

// Call sends messages to the OpenAI API and returns the generated response.
func (c *Client) Call(ctx context.Context, messages []map[string]string) (string, error) {
	reqBody := map[string]interface{}{
		"model":             c.Model,
		"temperature":       c.Temperature,
		"top_p":             c.TopP,
		"messages":          messages,
		"include_reasoning": true,
		"provider":          map[string]bool{"require_parameters": true},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.URL, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.Token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("non-200 response: %d - %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("unexpected response: no choices")
	}
	return result.Choices[0].Message.Content, nil
}
