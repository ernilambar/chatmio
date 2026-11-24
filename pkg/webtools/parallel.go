package webtools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ernilambar/chatmio/pkg/ai"
)

// parallelSearchDefinition mirrors Parallel's web_search arguments, which need
// an objective and at least one keyword query for good results.
func parallelSearchDefinition() ai.Tool {
	return ai.Tool{
		Type: "function",
		Function: ai.ToolFunction{
			Name:        SearchTool,
			Description: "Search the web for current information. Returns ranked titles, URLs and page excerpts that usually answer the question.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"objective": map[string]any{
						"type":        "string",
						"description": "One natural-language goal for the search",
					},
					"search_queries": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "1-3 short keyword queries of 3-6 words each",
					},
				},
				"required": []string{"objective", "search_queries"},
			},
		},
	}
}

// parallelResults is the payload Parallel returns as JSON text inside the MCP
// tool result, for both web_search and web_fetch.
type parallelResults struct {
	Results []struct {
		URL      string   `json:"url"`
		Title    string   `json:"title"`
		Excerpts []string `json:"excerpts"`
	} `json:"results"`
}

func (c *Client) searchParallel(ctx context.Context, arguments string) (string, string, error) {
	var args struct {
		Objective     string   `json:"objective"`
		Query         string   `json:"query"`
		SearchQueries []string `json:"search_queries"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", "", fmt.Errorf("webtools: decode arguments: %w", err)
	}
	objective := strings.TrimSpace(args.Objective)
	if objective == "" {
		objective = strings.TrimSpace(args.Query)
	}
	queries := make([]string, 0, len(args.SearchQueries))
	for _, q := range args.SearchQueries {
		if q = strings.TrimSpace(q); q != "" {
			queries = append(queries, q)
		}
	}
	if objective == "" && len(queries) == 0 {
		return "", "", errors.New("objective is required")
	}
	if objective == "" {
		objective = queries[0]
	}
	if len(queries) == 0 {
		queries = []string{objective}
	} else if len(queries) > 3 {
		queries = queries[:3]
	}

	text, err := c.parallelCall(ctx, SearchTool, map[string]any{
		"objective":      objective,
		"search_queries": queries,
	})
	if err != nil {
		return "", "", err
	}
	var payload parallelResults
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return "", "", fmt.Errorf("webtools: decode search response: %w", err)
	}
	if len(payload.Results) == 0 {
		return "No results.", "0 results", nil
	}
	if len(payload.Results) > searchResults {
		payload.Results = payload.Results[:searchResults]
	}

	parts := make([]string, 0, len(payload.Results))
	for i, r := range payload.Results {
		parts = append(parts, fmt.Sprintf("%d. %s\n%s\n%s",
			i+1, shortTitle(plainText(r.Title)), r.URL, truncate(strings.Join(r.Excerpts, "\n\n"), maxResultRunes)))
	}
	return strings.Join(parts, "\n\n"), resultSummary(len(payload.Results)), nil
}

func (c *Client) fetchParallel(ctx context.Context, rawURL string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", "", fmt.Errorf("webtools: invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", fmt.Errorf("webtools: unsupported url scheme %q", u.Scheme)
	}

	text, err := c.parallelCall(ctx, FetchTool, map[string]any{"urls": []string{u.String()}})
	if err != nil {
		return "", "", err
	}
	var payload parallelResults
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return "", "", fmt.Errorf("webtools: decode fetch response: %w", err)
	}
	if len(payload.Results) == 0 {
		return "", "", fmt.Errorf("webtools: no content for %s", u)
	}

	page := payload.Results[0]
	title := shortTitle(page.Title)
	res := truncate(strings.Join(page.Excerpts, "\n\n"), maxPageRunes)
	if title == "" {
		title = u.Host
	}
	if strings.TrimSpace(res) == "" {
		return "", "", fmt.Errorf("webtools: no readable content for %s", u)
	}
	return "# " + title + "\n\n" + res, title, nil
}

// parallelCall runs one MCP tool call. The anonymous endpoint accepts a bare
// tools/call, so no initialize handshake or session is needed.
func (c *Client) parallelCall(ctx context.Context, tool string, arguments any) (string, error) {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": tool, "arguments": arguments},
	})
	if err != nil {
		return "", fmt.Errorf("webtools: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("webtools: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("webtools: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("webtools: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return readSSE(io.LimitReader(resp.Body, maxResponseBytes))
	}

	var reply rpcReply
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&reply); err != nil {
		return "", fmt.Errorf("webtools: decode response: %w", err)
	}
	return reply.text()
}

// readSSE returns the text of the first JSON-RPC reply in an event stream.
func readSSE(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxResponseBytes)

	var data []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if text, ok, err := decodeSSEEvent(data); ok {
				return text, err
			}
			data = data[:0]
			continue
		}
		if payload, found := strings.CutPrefix(line, "data:"); found {
			data = append(data, strings.TrimPrefix(payload, " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("webtools: read response: %w", err)
	}
	if text, ok, err := decodeSSEEvent(data); ok {
		return text, err
	}
	return "", errors.New("webtools: empty response")
}

func decodeSSEEvent(data []string) (string, bool, error) {
	if len(data) == 0 {
		return "", false, nil
	}
	var reply rpcReply
	if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &reply); err != nil {
		return "", true, fmt.Errorf("webtools: decode response: %w", err)
	}
	if reply.Result == nil && reply.Error == nil {
		return "", false, nil
	}
	text, err := reply.text()
	return text, true, err
}

type rpcReply struct {
	Result *struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (r rpcReply) text() (string, error) {
	if r.Error != nil {
		return "", fmt.Errorf("webtools: %s", r.Error.Message)
	}
	if r.Result == nil {
		return "", errors.New("webtools: empty response")
	}
	var b strings.Builder
	for _, part := range r.Result.Content {
		b.WriteString(part.Text)
	}
	text := b.String()
	if r.Result.IsError {
		if strings.TrimSpace(text) == "" {
			text = "tool call failed"
		}
		return "", fmt.Errorf("webtools: %s", text)
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("webtools: empty response")
	}
	return text, nil
}
