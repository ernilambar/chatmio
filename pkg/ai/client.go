// Package ai implements an OpenAI-compatible chat client with SSE streaming.
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Role is a chat message role accepted by OpenAI-compatible APIs.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single chat message.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// ChatOptions is a streaming chat request.
type ChatOptions struct {
	Model            string
	Messages         []Message
	Temperature      *float64
	TopP             *float64
	MaxTokens        *int
	FrequencyPenalty *float64
}

// Client talks to one OpenAI-compatible endpoint.
type Client struct {
	BaseURL string
	APIKey  string
	Headers map[string]string
	HTTP    *http.Client
}

// New returns a client for baseURL (for example https://api.openai.com/v1).
func New(baseURL, apiKey string, headers map[string]string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Headers: headers,
		HTTP:    http.DefaultClient,
	}
}

type chatRequest struct {
	Model            string    `json:"model"`
	Messages         []Message `json:"messages"`
	Stream           bool      `json:"stream"`
	Temperature      *float64  `json:"temperature,omitempty"`
	TopP             *float64  `json:"top_p,omitempty"`
	MaxTokens        *int      `json:"max_tokens,omitempty"`
	FrequencyPenalty *float64  `json:"frequency_penalty,omitempty"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// StreamChat posts a streaming completion and calls onToken for each text delta.
// It returns nil after the stream sends [DONE].
func (c *Client) StreamChat(ctx context.Context, opts ChatOptions, onToken func(string)) error {
	if c.BaseURL == "" {
		return fmt.Errorf("ai: base URL is required")
	}
	if opts.Model == "" {
		return fmt.Errorf("ai: model is required")
	}

	payload, err := json.Marshal(chatRequest{
		Model:            opts.Model,
		Messages:         opts.Messages,
		Stream:           true,
		Temperature:      opts.Temperature,
		TopP:             opts.TopP,
		MaxTokens:        opts.MaxTokens,
		FrequencyPenalty: opts.FrequencyPenalty,
	})
	if err != nil {
		return fmt.Errorf("ai: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("ai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("ai: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			return nil
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("ai: decode stream chunk: %w", err)
		}
		if chunk.Error != nil {
			return fmt.Errorf("ai: %s", chunk.Error.Message)
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			onToken(chunk.Choices[0].Delta.Content)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("ai: read stream: %w", err)
	}
	return nil
}
