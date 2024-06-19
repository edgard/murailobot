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
func NewOpenAI(token, instruction string) *OpenAI {
	return &OpenAI{Token: token, Instruction: instruction}
}

// sendRequest sends a request to the OpenAI API and returns the response body and status code.
func (client *OpenAI) sendRequest(body map[string]interface{}) ([]byte, int, error) {
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.Token))

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response body: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

// Ping checks if the provided OpenAI token can connect to the OpenAI API.
func (client *OpenAI) Ping() error {
	pingMessage := map[string]interface{}{
		"model":       "gpt-4o",
		"messages":    []map[string]string{{"role": "system", "content": "ping"}},
		"temperature": 0.0,
	}

	_, statusCode, err := client.sendRequest(pingMessage)
	if err != nil {
		return err
	}

	if statusCode != http.StatusOK {
		return fmt.Errorf("failed to connect to OpenAI API: status %d", statusCode)
	}

	return nil
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
		return "", err
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(response.Choices) > 0 {
		return response.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("unexpected message format")
}
