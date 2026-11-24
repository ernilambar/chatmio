// Package webtools exposes web search and page fetch tools. Search runs on the
// Parallel MCP server, the Brave Search API, or the Ollama web API; Brave
// fetches pages locally while Parallel and Ollama fetch them through their own
// APIs.
package webtools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/ernilambar/chatmio/pkg/ai"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const (
	// ProviderParallel, ProviderBrave, and ProviderOllama are the supported
	// search providers.
	ProviderParallel = "parallel"
	ProviderBrave    = "brave"
	ProviderOllama   = "ollama"

	ParallelBaseURL = "https://mcp.parallel.ai/mcp"
	BraveBaseURL    = "https://api.search.brave.com/res/v1"
	OllamaBaseURL   = "https://ollama.com/api"

	SearchTool = "web_search"
	FetchTool  = "web_fetch"

	searchResults    = 2
	maxResultRunes   = 1000
	maxTitleRunes    = 200
	maxPageRunes     = 8000
	maxResponseBytes = 4 << 20
	requestTimeout   = 30 * time.Second
	userAgent        = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"
)

// Definitions returns the tools offered to the model. Brave and Ollama take a
// single search query; use Client.Definitions for the provider's own shape.
func Definitions() []ai.Tool {
	return []ai.Tool{searchDefinition(), fetchDefinition()}
}

// Definitions returns the tool definitions for the client's provider. Parallel
// takes an objective plus keyword queries, so its search schema differs.
func (c *Client) Definitions() []ai.Tool {
	if c.Provider == ProviderParallel {
		return []ai.Tool{parallelSearchDefinition(), fetchDefinition()}
	}
	return Definitions()
}

func searchDefinition() ai.Tool {
	return ai.Tool{
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
	}
}

func fetchDefinition() ai.Tool {
	return ai.Tool{
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
	}
}

// Client calls the configured search provider.
type Client struct {
	Provider string
	BaseURL  string
	APIKey   string
	HTTP     *http.Client

	// fetchHTTP fetches model-chosen pages. Its dialer connects only to the
	// addresses it has checked, so DNS rebinding cannot reach private hosts.
	fetchHTTP *http.Client
	// lookup resolves hostnames; tests replace it.
	lookup func(ctx context.Context, host string) ([]netip.Addr, error)
	// allowPrivate lets tests fetch loopback servers.
	allowPrivate bool
}

// NormalizeProvider maps a stored search provider name to a supported one.
// Unknown or empty values select Parallel, which works without a key.
func NormalizeProvider(provider string) string {
	switch provider {
	case ProviderBrave, ProviderOllama:
		return provider
	default:
		return ProviderParallel
	}
}

// New returns a client for provider. Parallel works without an API key on its
// free anonymous tier.
func New(provider, apiKey string) *Client {
	provider = NormalizeProvider(provider)
	base := ParallelBaseURL
	switch provider {
	case ProviderOllama:
		base = OllamaBaseURL
	case ProviderBrave:
		base = BraveBaseURL
	}
	c := &Client{Provider: provider, BaseURL: base, APIKey: apiKey, lookup: lookupHost}
	checkRedirect := func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if c.allowPrivate {
			return nil
		}
		return c.checkPublic(req.Context(), req.URL)
	}
	c.HTTP = &http.Client{Timeout: requestTimeout, CheckRedirect: checkRedirect}

	// No proxy: the dialer must see the page's own address, not the proxy's.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = c.dialPublic
	c.fetchHTTP = &http.Client{Timeout: requestTimeout, CheckRedirect: checkRedirect, Transport: transport}
	return c
}

// Input returns the query, objective, or URL from a call's raw JSON arguments,
// for display.
func Input(arguments string) string {
	var args struct {
		Query     string `json:"query"`
		Objective string `json:"objective"`
		URL       string `json:"url"`
	}
	_ = json.Unmarshal([]byte(arguments), &args)
	if args.Query != "" {
		return strings.TrimSpace(args.Query)
	}
	if args.Objective != "" {
		return strings.TrimSpace(args.Objective)
	}
	return strings.TrimSpace(args.URL)
}

// Run executes one tool call and returns the text sent back to the model and a
// short summary for the UI.
func (c *Client) Run(ctx context.Context, name, arguments string) (result, summary string, err error) {
	switch name {
	case SearchTool:
		if c.Provider == ProviderParallel {
			return c.searchParallel(ctx, arguments)
		}
		input := Input(arguments)
		if input == "" {
			return "", "", errors.New("query is required")
		}
		if c.Provider == ProviderOllama {
			return c.searchOllama(ctx, input)
		}
		return c.searchBrave(ctx, input)
	case FetchTool:
		input := Input(arguments)
		if input == "" {
			return "", "", errors.New("url is required")
		}
		switch c.Provider {
		case ProviderParallel:
			return c.fetchParallel(ctx, input)
		case ProviderOllama:
			return c.fetchOllama(ctx, input)
		default:
			return c.fetchBrave(ctx, input)
		}
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
	if len(payload.Web.Results) > searchResults {
		payload.Web.Results = payload.Web.Results[:searchResults]
	}

	parts := make([]string, 0, len(payload.Web.Results))
	for i, r := range payload.Web.Results {
		parts = append(parts, fmt.Sprintf("%d. %s\n%s\n%s",
			i+1, shortTitle(plainText(r.Title)), r.URL, truncate(plainText(r.Description), maxResultRunes)))
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
	if len(payload.Results) > searchResults {
		payload.Results = payload.Results[:searchResults]
	}

	parts := make([]string, 0, len(payload.Results))
	for i, r := range payload.Results {
		parts = append(parts, fmt.Sprintf("%d. %s\n%s\n%s",
			i+1, shortTitle(r.Title), r.URL, truncate(r.Content, maxResultRunes)))
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
		if err := c.checkPublic(ctx, u); err != nil {
			return "", "", err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", "", fmt.Errorf("webtools: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.1")

	resp, err := c.fetchHTTP.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("webtools: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", "", fmt.Errorf("webtools: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	contentType := resp.Header.Get("Content-Type")
	// Media types are case-insensitive; ParseMediaType lowercases them.
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.ToLower(strings.TrimSpace(contentType))
	}
	body := io.LimitReader(resp.Body, maxResponseBytes)
	decoded, err := charset.NewReader(body, contentType)
	if err != nil {
		return "", "", fmt.Errorf("webtools: decode page: %w", err)
	}

	var title, text string
	switch {
	case mediaType == "" || strings.Contains(mediaType, "html"):
		title, text, err = htmlText(decoded)
	case strings.HasPrefix(mediaType, "text/"):
		var data []byte
		data, err = io.ReadAll(decoded)
		text = string(data)
	default:
		return "", "", fmt.Errorf("webtools: unsupported content type %q", contentType)
	}
	if err != nil {
		return "", "", fmt.Errorf("webtools: read page: %w", err)
	}

	title = shortTitle(title)
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
	title := shortTitle(payload.Title)
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
// It fails early; dialPublic enforces the same rule on the actual connection.
func (c *Client) checkPublic(ctx context.Context, u *url.URL) error {
	host := u.Hostname()
	if host == "" {
		return errors.New("webtools: url has no host")
	}
	_, err := c.resolvePublic(ctx, host)
	return err
}

// dialPublic resolves the host once, refuses any non-public address, and dials
// the checked address itself so a second DNS answer cannot redirect it.
func (c *Client) dialPublic(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: requestTimeout, KeepAlive: 30 * time.Second}
	if c.allowPrivate {
		return dialer.DialContext(ctx, network, address)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("webtools: invalid address %q: %w", address, err)
	}
	addrs, err := c.resolvePublic(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, addr := range addrs {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// resolvePublic returns host's addresses, or an error if any is not public.
func (c *Client) resolvePublic(ctx context.Context, host string) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		addr = addr.Unmap()
		if !isPublicAddr(addr) {
			return nil, fmt.Errorf("webtools: refusing to fetch non-public address %s", addr)
		}
		return []netip.Addr{addr}, nil
	}
	addrs, err := c.lookup(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("webtools: resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("webtools: resolve %s: no addresses", host)
	}
	for i, addr := range addrs {
		addrs[i] = addr.Unmap()
		if !isPublicAddr(addrs[i]) {
			return nil, fmt.Errorf("webtools: refusing to fetch non-public address %s", host)
		}
	}
	return addrs, nil
}

func lookupHost(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// nonPublicPrefixes are the IANA special-purpose ranges that are not globally
// reachable, plus translation ranges that can embed a private IPv4 address.
var nonPublicPrefixes = func() []netip.Prefix {
	ranges := []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
		"172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16",
		"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
		"::/128", "::1/128", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/23",
		"2001:db8::/32", "2002::/16", "3fff::/20", "5f00::/16", "fc00::/7", "fe80::/10", "ff00::/8",
	}
	prefixes := make([]netip.Prefix, len(ranges))
	for i, r := range ranges {
		prefixes[i] = netip.MustParsePrefix(r)
	}
	return prefixes
}()

// isPublicAddr reports whether addr is globally routable. Callers pass
// unmapped addresses so IPv4-mapped IPv6 is checked as IPv4.
func isPublicAddr(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
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

// shortTitle trims a page or result title to maxTitleRunes, so a huge <title>
// cannot get past the page output limit.
func shortTitle(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxTitleRunes {
		return s
	}
	return string(runes[:maxTitleRunes]) + "…"
}

func truncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "\n[truncated]"
}
