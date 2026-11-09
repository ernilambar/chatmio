// Package webtools exposes web search and page fetch tools backed by the
// Ollama web search API.
package webtools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ernilambar/chattu/pkg/ai"
)

const (
	// DefaultBaseURL is the Ollama web API root.
	DefaultBaseURL = "https://ollama.com/api"

	SearchTool = "web_search"
	FetchTool  = "web_fetch"

	searchResults    = 2
	maxResultRunes   = 1000
	maxPageRunes     = 8000
	maxResponseBytes = 4 << 20
	requestTimeout   = 30 * time.Second
)

// Definitions returns the tools offered to the model.
func Definitions() []ai.Tool {
	return []ai.Tool{
		{
			Type: "function",
			Function: ai.ToolFunction{
				Name:        SearchTool,
				Description: "Search the web for current information. Returns titles, URLs and page excerpts.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string", "description": "Search query"},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: ai.ToolFunction{
				Name:        FetchTool,
				Description: "Fetch a web page by URL and return its readable text.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{"type": "string", "description": "Absolute http(s) URL"},
					},
					"required": []string{"url"},
				},
			},
		},
	}
}

// Client calls the Ollama web API.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// New returns a client for the Ollama web API.
func New(apiKey string) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: requestTimeout},
	}
}

// Input returns the query or URL from a call's raw JSON arguments, for display.
func Input(arguments string) string {
	var args struct {
		Query string `json:"query"`
		URL   string `json:"url"`
	}
	_ = json.Unmarshal([]byte(arguments), &args)
	if args.Query != "" {
		return strings.TrimSpace(args.Query)
	}
	return strings.TrimSpace(args.URL)
}

// Run executes one tool call and returns the text sent back to the model and a
// short summary for the UI.
func (c *Client) Run(ctx context.Context, name, arguments string) (result, summary string, err error) {
	input := Input(arguments)
	switch name {
	case SearchTool:
		if input == "" {
			return "", "", errors.New("query is required")
		}
		return c.search(ctx, input)
	case FetchTool:
		if input == "" {
			return "", "", errors.New("url is required")
		}
		return c.fetch(ctx, input)
	default:
		return "", "", fmt.Errorf("unknown tool %q", name)
	}
}

func (c *Client) search(ctx context.Context, query string) (string, string, error) {
	var payload struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := c.post(ctx, "/web_search", map[string]any{"query": query, "max_results": searchResults}, &payload); err != nil {
		return "", "", err
	}
	if len(payload.Results) == 0 {
		return "No results.", "0 results", nil
	}

	parts := make([]string, 0, len(payload.Results))
	for i, r := range payload.Results {
		parts = append(parts, fmt.Sprintf("%d. %s\n%s\n%s", i+1, r.Title, r.URL, truncate(r.Content, maxResultRunes)))
	}
	summary := fmt.Sprintf("%d results", len(payload.Results))
	if len(payload.Results) == 1 {
		summary = "1 result"
	}
	return strings.Join(parts, "\n\n"), summary, nil
}

func (c *Client) fetch(ctx context.Context, url string) (string, string, error) {
	var payload struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := c.post(ctx, "/web_fetch", map[string]any{"url": url}, &payload); err != nil {
		return "", "", err
	}
	title := strings.TrimSpace(payload.Title)
	result := truncate(payload.Content, maxPageRunes)
	if title != "" {
		result = "# " + title + "\n\n" + result
	}
	return result, title, nil
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("webtools: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("webtools: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("webtools: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("webtools: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return fmt.Errorf("webtools: decode response: %w", err)
	}
	return nil
}

func truncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "\n[truncated]"
}
