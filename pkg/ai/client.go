// Package ai implements an OpenAI-compatible chat client with SSE streaming.
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// Role is a chat message role accepted by OpenAI-compatible APIs.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a single chat message. Content is either a string or a
// []ContentPart for messages that include images.
type Message struct {
	Role    Role `json:"role"`
	Content any  `json:"content"`
	// ToolCalls is set on assistant messages that request tools.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID links a tool result message to the call it answers.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// Tool is a function the model may call.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable function with a JSON Schema for its
// arguments.
type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
}

// ToolCall is one function call requested by the model. Arguments is the raw
// JSON string the model produced.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction is the function name and arguments of a ToolCall.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
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
	// Tools are offered to the model; nil sends no tools.
	Tools []Tool
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

// DefaultIdleTimeout is how long a stream may go without receiving any data
// before the request is abandoned.
const DefaultIdleTimeout = 2 * time.Minute

// ErrStreamStalled reports that a provider stopped sending data for longer
// than the client's idle timeout.
var ErrStreamStalled = errors.New("ai: provider stopped sending data")

// Client talks to one OpenAI-compatible endpoint.
type Client struct {
	BaseURL string
	APIKey  string
	Headers map[string]string
	HTTP    *http.Client
	// IdleTimeout bounds how long a request or stream may go without data.
	// Zero uses DefaultIdleTimeout.
	IdleTimeout time.Duration
}

// New returns a client for baseURL (for example https://api.openai.com/v1).
func New(baseURL, apiKey string, headers map[string]string) *Client {
	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		APIKey:      apiKey,
		Headers:     headers,
		HTTP:        http.DefaultClient,
		IdleTimeout: DefaultIdleTimeout,
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

// StatusError is a non-success HTTP response from the provider.
type StatusError struct {
	StatusCode int
	Status     string
	Message    string
}

func (e *StatusError) Error() string {
	if e.Message == "" {
		return "ai: " + e.Status
	}
	return "ai: " + e.Status + ": " + e.Message
}

// apiError describes a failed response. Providers usually wrap the reason in
// {"error":{"message":"…"}}, so prefer that over the raw JSON body.
func apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := strings.TrimSpace(string(body))
	var payload struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Error != nil {
		if msg := strings.TrimSpace(payload.Error.Message); msg != "" {
			message = msg
		}
	}
	return &StatusError{StatusCode: resp.StatusCode, Status: resp.Status, Message: message}
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
		return nil, apiError(resp)
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
	Tools            []Tool    `json:"tools,omitempty"`
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
			ToolCalls        []struct {
				Index    *int   `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
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

// Reply is the result of a finished stream.
type Reply struct {
	Usage Usage
	// ToolCalls are the complete tool calls the model requested, if any.
	ToolCalls []ToolCall
}

// StreamChat posts a streaming completion and calls onDelta for each content
// or reasoning increment. It returns the reported usage and any requested tool
// calls after the stream ends.
func (c *Client) StreamChat(ctx context.Context, opts ChatOptions, onDelta func(Delta)) (Reply, error) {
	var reply Reply
	if c.BaseURL == "" {
		return reply, fmt.Errorf("ai: base URL is required")
	}
	if opts.Model == "" {
		return reply, fmt.Errorf("ai: model is required")
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
		Tools:            opts.Tools,
	}
	request.StreamOptions.IncludeUsage = true

	payload, err := json.Marshal(request)
	if err != nil {
		return reply, fmt.Errorf("ai: encode request: %w", err)
	}

	idle := c.IdleTimeout
	if idle <= 0 {
		idle = DefaultIdleTimeout
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var lastActivity atomic.Int64
	var stalled atomic.Bool
	lastActivity.Store(time.Now().UnixNano())
	go watchIdle(streamCtx, idle, &lastActivity, &stalled, cancel)

	req, err := http.NewRequestWithContext(streamCtx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return reply, fmt.Errorf("ai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	c.setHeaders(req)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return reply, streamError(err, &stalled, idle)
	}
	defer resp.Body.Close()
	lastActivity.Store(time.Now().UnixNano())

	if resp.StatusCode != http.StatusOK {
		return reply, apiError(resp)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lastActivity.Store(time.Now().UnixNano())
		line := strings.TrimSpace(scanner.Text())
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			return reply, nil
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return reply, fmt.Errorf("ai: decode stream chunk: %w", err)
		}
		if chunk.Error != nil {
			return reply, fmt.Errorf("ai: %s", chunk.Error.Message)
		}
		if chunk.Usage != nil {
			reply.Usage.PromptTokens = chunk.Usage.PromptTokens
			reply.Usage.CompletionTokens = chunk.Usage.CompletionTokens
			if details := chunk.Usage.CompletionTokensDetails; details != nil {
				reply.Usage.ReasoningTokens = details.ReasoningTokens
			}
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		for _, part := range delta.ToolCalls {
			reply.ToolCalls = mergeToolCall(reply.ToolCalls, part.Index, part.ID, part.Function.Name, part.Function.Arguments)
		}
		reasoning := delta.ReasoningContent
		if reasoning == "" {
			reasoning = delta.Reasoning
		}
		if delta.Content != "" || reasoning != "" {
			onDelta(Delta{Content: delta.Content, Reasoning: reasoning})
		}
	}
	if stalled.Load() {
		return reply, streamError(context.Canceled, &stalled, idle)
	}
	if err := scanner.Err(); err != nil {
		return reply, fmt.Errorf("ai: read stream: %w", err)
	}
	return reply, nil
}

// watchIdle cancels the request when no data arrives for longer than idle.
func watchIdle(ctx context.Context, idle time.Duration, last *atomic.Int64, stalled *atomic.Bool, cancel context.CancelFunc) {
	interval := idle / 4
	if interval > time.Second {
		interval = time.Second
	}
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Since(time.Unix(0, last.Load())) >= idle {
				stalled.Store(true)
				cancel()
				return
			}
		}
	}
}

// streamError turns a cancellation caused by the idle watchdog into a
// recognisable error while leaving user and shutdown cancellations intact.
func streamError(err error, stalled *atomic.Bool, idle time.Duration) error {
	if stalled.Load() {
		return fmt.Errorf("%w (no data for %s)", ErrStreamStalled, idle)
	}
	if err == context.Canceled {
		return err
	}
	return fmt.Errorf("ai: request failed: %w", err)
}

// mergeToolCall folds one streamed tool-call fragment into calls. Fragments
// share an index; providers that omit it start a new call with each new id.
func mergeToolCall(calls []ToolCall, index *int, id, name, arguments string) []ToolCall {
	i := len(calls) - 1
	switch {
	case index != nil:
		i = *index
	case id != "" && (i < 0 || calls[i].ID != id):
		i = len(calls)
	}
	if i < 0 {
		i = 0
	}
	for len(calls) <= i {
		calls = append(calls, ToolCall{Type: "function"})
	}
	if id != "" {
		calls[i].ID = id
	}
	if calls[i].Function.Name == "" {
		calls[i].Function.Name = name
	}
	calls[i].Function.Arguments += arguments
	return calls
}
