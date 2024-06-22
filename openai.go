package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OpenAI encapsulates the logic for interacting with the OpenAI API.
type OpenAI struct {
	Token       string
	Instruction string
}

// NewOpenAI creates a new OpenAI client.
func NewOpenAI(config *Config) (*OpenAI, error) {
	if config.OpenAIToken == "" || config.OpenAIInstruction == "" {
		return nil, WrapError(fmt.Errorf("invalid OpenAI configuration"))
	}
	return &OpenAI{Token: config.OpenAIToken, Instruction: config.OpenAIInstruction}, nil
}

// sendRequest sends a request to the OpenAI API and returns the response body and status code.
func (client *OpenAI) sendRequest(body map[string]interface{}) ([]byte, int, error) {
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, WrapError(fmt.Errorf("failed to marshal request body: %w", err))
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, 0, WrapError(fmt.Errorf("failed to create request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.Token))

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, WrapError(fmt.Errorf("failed to send request: %w", err))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, WrapError(fmt.Errorf("failed to read response body: %w", err))
	}

	return respBody, resp.StatusCode, nil
}

// Call sends a request to the OpenAI API and returns the response.
func (client *OpenAI) Call(messages []map[string]string, temperature float32) (string, error) {
	requestBody := map[string]interface{}{
		"model":       "gpt-4o",
		"messages":    messages,
		"temperature": temperature,
	}

	respBody, _, err := client.sendRequest(requestBody)
	if err != nil {
		return "", WrapError(fmt.Errorf("call to OpenAI API failed: %w", err))
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		return "", WrapError(fmt.Errorf("failed to unmarshal response: %w", err))
	}

	if len(response.Choices) > 0 {
		return response.Choices[0].Message.Content, nil
	}

	return "", WrapError(fmt.Errorf("unexpected message format: no choices in response"))
}
