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
	"sort"
	"strings"
)

// Role is a chat message role accepted by OpenAI-compatible APIs.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single chat message. Content is either a string or a
// []ContentPart for messages that include images.
type Message struct {
	Role    Role `json:"role"`
	Content any  `json:"content"`
}

// ContentPart is one item in a multimodal message.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL points at an image supplied as a data URL.
type ImageURL struct {
	URL string `json:"url"`
}

// TextPart returns a text content part.
func TextPart(text string) ContentPart {
	return ContentPart{Type: "text", Text: text}
}

// ImagePart returns an image content part.
func ImagePart(url string) ContentPart {
	return ContentPart{Type: "image_url", ImageURL: &ImageURL{URL: url}}
}

// Model is one entry returned by GET /models.
type Model struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by"`
}

// ChatOptions is a streaming chat request.
type ChatOptions struct {
	Model            string
	Messages         []Message
	Temperature      *float64
	TopP             *float64
	MaxTokens        *int
	FrequencyPenalty *float64
	// ReasoningEffort is sent as reasoning_effort when set (low, medium, high).
	ReasoningEffort string
}

// Delta is one streamed increment. Reasoning carries thinking text from
// providers that expose it as reasoning_content or reasoning.
type Delta struct {
	Content   string
	Reasoning string
}

// Usage is the token accounting reported at the end of a stream. It is zero
// when the provider does not report usage.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	ReasoningTokens  int
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

func (c *Client) setHeaders(req *http.Request) {
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}
}

// ListModels returns the model identifiers exposed by GET /models.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("ai: base URL is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("ai: build models request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	c.setHeaders(req)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai: models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("ai: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Data []Model `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("ai: decode models: %w", err)
	}

	seen := make(map[string]struct{}, len(payload.Data))
	ids := make([]string, 0, len(payload.Data))
	for _, model := range payload.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

type chatRequest struct {
	Model            string    `json:"model"`
	Messages         []Message `json:"messages"`
	Stream           bool      `json:"stream"`
	Temperature      *float64  `json:"temperature,omitempty"`
	TopP             *float64  `json:"top_p,omitempty"`
	MaxTokens        *int      `json:"max_tokens,omitempty"`
	FrequencyPenalty *float64  `json:"frequency_penalty,omitempty"`
	ReasoningEffort  string    `json:"reasoning_effort,omitempty"`
	StreamOptions    struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens            int `json:"prompt_tokens"`
		CompletionTokens        int `json:"completion_tokens"`
		CompletionTokensDetails *struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// StreamChat posts a streaming completion and calls onDelta for each content
// or reasoning increment. It returns the reported usage after the stream ends.
func (c *Client) StreamChat(ctx context.Context, opts ChatOptions, onDelta func(Delta)) (Usage, error) {
	var usage Usage
	if c.BaseURL == "" {
		return usage, fmt.Errorf("ai: base URL is required")
	}
	if opts.Model == "" {
		return usage, fmt.Errorf("ai: model is required")
	}

	request := chatRequest{
		Model:            opts.Model,
		Messages:         opts.Messages,
		Stream:           true,
		Temperature:      opts.Temperature,
		TopP:             opts.TopP,
		MaxTokens:        opts.MaxTokens,
		FrequencyPenalty: opts.FrequencyPenalty,
		ReasoningEffort:  opts.ReasoningEffort,
	}
	request.StreamOptions.IncludeUsage = true

	payload, err := json.Marshal(request)
	if err != nil {
		return usage, fmt.Errorf("ai: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return usage, fmt.Errorf("ai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	c.setHeaders(req)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return usage, fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return usage, fmt.Errorf("ai: %s: %s", resp.Status, strings.TrimSpace(string(body)))
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
			return usage, nil
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return usage, fmt.Errorf("ai: decode stream chunk: %w", err)
		}
		if chunk.Error != nil {
			return usage, fmt.Errorf("ai: %s", chunk.Error.Message)
		}
		if chunk.Usage != nil {
			usage.PromptTokens = chunk.Usage.PromptTokens
			usage.CompletionTokens = chunk.Usage.CompletionTokens
			if details := chunk.Usage.CompletionTokensDetails; details != nil {
				usage.ReasoningTokens = details.ReasoningTokens
			}
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		reasoning := delta.ReasoningContent
		if reasoning == "" {
			reasoning = delta.Reasoning
		}
		if delta.Content != "" || reasoning != "" {
			onDelta(Delta{Content: delta.Content, Reasoning: reasoning})
		}
	}
	if err := scanner.Err(); err != nil {
		return usage, fmt.Errorf("ai: read stream: %w", err)
	}
	return usage, nil
}
