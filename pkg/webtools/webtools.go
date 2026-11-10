// Package webtools exposes web search and page fetch tools. Search runs on the
// Brave Search API or the Ollama web API; Brave fetches pages locally while
// Ollama fetches them through its own API.
package webtools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ernilambar/chatmio/pkg/ai"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const (
	// ProviderBrave and ProviderOllama are the supported search providers.
	ProviderBrave  = "brave"
	ProviderOllama = "ollama"

	BraveBaseURL  = "https://api.search.brave.com/res/v1"
	OllamaBaseURL = "https://ollama.com/api"

	SearchTool = "web_search"
	FetchTool  = "web_fetch"

	searchResults    = 2
	maxResultRunes   = 1000
	maxPageRunes     = 8000
	maxResponseBytes = 4 << 20
	requestTimeout   = 30 * time.Second
	userAgent        = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"
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

// Client calls the configured search provider.
type Client struct {
	Provider string
	BaseURL  string
	APIKey   string
	HTTP     *http.Client

	// allowPrivate lets tests fetch loopback servers.
	allowPrivate bool
}

// New returns a client for provider. Any value other than ProviderOllama uses
// the Brave Search API.
func New(provider, apiKey string) *Client {
	if provider != ProviderOllama {
		provider = ProviderBrave
	}
	base := BraveBaseURL
	if provider == ProviderOllama {
		base = OllamaBaseURL
	}
	c := &Client{Provider: provider, BaseURL: base, APIKey: apiKey}
	c.HTTP = &http.Client{
		Timeout: requestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if c.allowPrivate {
				return nil
			}
			return checkPublic(req.Context(), req.URL)
		},
	}
	return c
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
		if c.Provider == ProviderOllama {
			return c.searchOllama(ctx, input)
		}
		return c.searchBrave(ctx, input)
	case FetchTool:
		if input == "" {
			return "", "", errors.New("url is required")
		}
		if c.Provider == ProviderOllama {
			return c.fetchOllama(ctx, input)
		}
		return c.fetchBrave(ctx, input)
	default:
		return "", "", fmt.Errorf("unknown tool %q", name)
	}
}

func (c *Client) searchBrave(ctx context.Context, query string) (string, string, error) {
	params := url.Values{
		"q":     {query},
		"count": {fmt.Sprint(searchResults)},
	}
	var payload struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := c.get(ctx, "/web/search?"+params.Encode(), &payload); err != nil {
		return "", "", err
	}
	if len(payload.Web.Results) == 0 {
		return "No results.", "0 results", nil
	}

	parts := make([]string, 0, len(payload.Web.Results))
	for i, r := range payload.Web.Results {
		parts = append(parts, fmt.Sprintf("%d. %s\n%s\n%s",
			i+1, plainText(r.Title), r.URL, truncate(plainText(r.Description), maxResultRunes)))
	}
	return strings.Join(parts, "\n\n"), resultSummary(len(payload.Web.Results)), nil
}

func (c *Client) searchOllama(ctx context.Context, query string) (string, string, error) {
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
		parts = append(parts, fmt.Sprintf("%d. %s\n%s\n%s",
			i+1, r.Title, r.URL, truncate(r.Content, maxResultRunes)))
	}
	return strings.Join(parts, "\n\n"), resultSummary(len(payload.Results)), nil
}

func (c *Client) fetchBrave(ctx context.Context, rawURL string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", "", fmt.Errorf("webtools: invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", fmt.Errorf("webtools: unsupported url scheme %q", u.Scheme)
	}
	if !c.allowPrivate {
		if err := checkPublic(ctx, u); err != nil {
			return "", "", err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", "", fmt.Errorf("webtools: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.1")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("webtools: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", "", fmt.Errorf("webtools: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	contentType := resp.Header.Get("Content-Type")
	body := io.LimitReader(resp.Body, maxResponseBytes)
	decoded, err := charset.NewReader(body, contentType)
	if err != nil {
		return "", "", fmt.Errorf("webtools: decode page: %w", err)
	}

	var title, text string
	switch {
	case contentType == "" || strings.Contains(contentType, "html"):
		title, text, err = htmlText(decoded)
	case strings.HasPrefix(contentType, "text/"):
		var data []byte
		data, err = io.ReadAll(decoded)
		text = string(data)
	default:
		return "", "", fmt.Errorf("webtools: unsupported content type %q", contentType)
	}
	if err != nil {
		return "", "", fmt.Errorf("webtools: read page: %w", err)
	}

	if title == "" {
		title = u.Host
	}
	result := truncate(text, maxPageRunes)
	if title != "" {
		result = "# " + title + "\n\n" + result
	}
	return result, title, nil
}

func (c *Client) fetchOllama(ctx context.Context, url string) (string, string, error) {
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

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("webtools: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", c.APIKey)

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

func resultSummary(n int) string {
	if n == 1 {
		return "1 result"
	}
	return fmt.Sprintf("%d results", n)
}

// checkPublic rejects URLs that resolve to loopback or private addresses so a
// model-generated URL cannot reach services on the user's machine or network.
func checkPublic(ctx context.Context, u *url.URL) error {
	host := u.Hostname()
	if host == "" {
		return errors.New("webtools: url has no host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return fmt.Errorf("webtools: refusing to fetch non-public address %s", ip)
		}
		return nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("webtools: resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("webtools: resolve %s: no addresses", host)
	}
	for _, addr := range addrs {
		if !isPublicIP(addr.IP) {
			return fmt.Errorf("webtools: refusing to fetch non-public address %s", host)
		}
	}
	return nil
}

func isPublicIP(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() && !ip.IsUnspecified() && !ip.IsMulticast()
}

var blockTags = map[string]bool{
	"address": true, "article": true, "blockquote": true, "br": true, "dd": true,
	"div": true, "dl": true, "dt": true, "figcaption": true, "figure": true,
	"footer": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true,
	"h6": true, "header": true, "hr": true, "li": true, "main": true, "nav": true,
	"ol": true, "p": true, "pre": true, "section": true, "table": true, "td": true,
	"th": true, "tr": true, "ul": true,
}

var skipTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "template": true, "svg": true,
}

// htmlText extracts the title and readable text from a page.
func htmlText(r io.Reader) (title, text string, err error) {
	z := html.NewTokenizer(r)
	var titleBuf, textBuf strings.Builder
	var skip []string
	inTitle := false

	for {
		switch z.Next() {
		case html.ErrorToken:
			if err := z.Err(); err != nil && err != io.EOF {
				return "", "", err
			}
			return normalizeSpace(titleBuf.String()), normalizeText(textBuf.String()), nil
		case html.StartTagToken, html.SelfClosingTagToken:
			name, selfClosing := z.TagName()
			tag := string(name)
			if skipTags[tag] {
				if !selfClosing {
					skip = append(skip, tag)
				}
				continue
			}
			if tag == "title" {
				inTitle = true
			}
			if blockTags[tag] {
				textBuf.WriteString("\n")
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if n := len(skip); n > 0 && skip[n-1] == tag {
				skip = skip[:n-1]
				continue
			}
			if tag == "title" {
				inTitle = false
			}
			if blockTags[tag] {
				textBuf.WriteString("\n")
			}
		case html.TextToken:
			if len(skip) > 0 {
				continue
			}
			if inTitle {
				titleBuf.WriteString(string(z.Text()))
			} else {
				textBuf.WriteString(string(z.Text()))
			}
		}
	}
}

// plainText strips markup and collapses whitespace, for one-line snippets.
func plainText(s string) string {
	_, text, err := htmlText(strings.NewReader(s))
	if err != nil {
		return strings.TrimSpace(html.UnescapeString(s))
	}
	return strings.Join(strings.Fields(text), " ")
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func normalizeText(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func truncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "\n[truncated]"
}
