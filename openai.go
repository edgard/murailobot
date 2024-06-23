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
	Token       string  // OpenAI API token
	Instruction string  // Instruction for OpenAI
	Model       string  // Model name for OpenAI
	Temperature float32 // Temperature setting for OpenAI
	TopP        float32 // TopP setting for OpenAI
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

// sendRequest sends a request to the OpenAI API and returns the response body.
func (client *OpenAI) sendRequest(body map[string]interface{}) ([]byte, error) {
	// Marshal the request body to JSON
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, WrapError("failed to marshal request body", err)
	}

	// Create a new HTTP request
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, WrapError("failed to create request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.Token))

	// Send the HTTP request
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, WrapError("failed to send request", err)
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, WrapError("failed to read response body", err)
	}

	return respBody, nil
}

// Call sends a request to the OpenAI API and returns the response.
func (client *OpenAI) Call(messages []map[string]string) (string, error) {
	// Prepare the request body
	requestBody := map[string]interface{}{
		"model":       client.Model,
		"temperature": client.Temperature,
		"top_p":       client.TopP,
		"messages":    messages,
	}

	// Send the request
	respBody, err := client.sendRequest(requestBody)
	if err != nil {
		return "", WrapError("call to OpenAI API failed", err)
	}

	// Parse the response
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	err = json.Unmarshal(respBody, &response)
	if err != nil {
		return "", WrapError("failed to unmarshal response", err)
	}

	// Extract the message content
	if len(response.Choices) > 0 {
		return response.Choices[0].Message.Content, nil
	}

	return "", WrapError("unexpected message format: no choices in response")
}
