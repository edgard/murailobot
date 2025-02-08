package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAI encapsulates the logic for interacting with the OpenAI API.
type OpenAI struct {
	Token       string  // OpenAI API token.
	Instruction string  // System instruction.
	Model       string  // Model name.
	Temperature float32 // Temperature parameter.
	TopP        float32 // TopP parameter.
}

// NewOpenAI creates a new OpenAI client.
func NewOpenAI(config *Config) (*OpenAI, error) {
	if config.OpenAIToken == "" || config.OpenAIInstruction == "" {
		return nil, WrapError("invalid OpenAI configuration")
	}
	return &OpenAI{
		Token:       config.OpenAIToken,
		Instruction: config.OpenAIInstruction,
		Model:       config.OpenAIModel,
		Temperature: config.OpenAITemperature,
		TopP:        config.OpenAITopP,
	}, nil
}

// sendRequest sends a JSON request to the OpenAI API and returns the response.
func (client *OpenAI) sendRequest(body map[string]interface{}) ([]byte, error) {
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, WrapError("failed to marshal request body", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, WrapError("failed to create request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.Token))

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, WrapError("failed to send request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, WrapError(fmt.Sprintf("non-200 response from OpenAI: %d - %s", resp.StatusCode, string(respBody)))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, WrapError("failed to read response body", err)
	}
	return respBody, nil
}

// Call sends the provided messages to OpenAI and returns the generated response.
func (client *OpenAI) Call(messages []map[string]string) (string, error) {
	requestBody := map[string]interface{}{
		"model":       client.Model,
		"temperature": client.Temperature,
		"top_p":       client.TopP,
		"messages":    messages,
	}

	respBody, err := client.sendRequest(requestBody)
	if err != nil {
		return "", WrapError("call to OpenAI API failed", err)
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		return "", WrapError("failed to unmarshal response", err)
	}

	if len(response.Choices) > 0 {
		return response.Choices[0].Message.Content, nil
	}

	return "", WrapError("unexpected message format: no choices in response")
}
