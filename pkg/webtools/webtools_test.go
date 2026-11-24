package webtools

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestRunSearchAndFetch(t *testing.T) {
	page := `<html><head><title>Go &amp; Web</title><style>.x{color:red}</style>
		<script>hidden()</script></head>
		<body><h1>Go</h1><p>The <strong>Go</strong> language.</p>
		<p>` + strings.Repeat("a", maxPageRunes) + `</p></body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/web/search":
			if r.Header.Get("X-Subscription-Token") != "key" {
				t.Errorf("X-Subscription-Token = %q", r.Header.Get("X-Subscription-Token"))
			}
			if q := r.URL.Query().Get("q"); q != "go" {
				t.Errorf("q = %q", q)
			}
			if n := r.URL.Query().Get("count"); n != "2" {
				t.Errorf("count = %q", n)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"web":{"results":[{"title":"Go <strong>lang</strong>","url":"https://go.dev","description":"The <strong>Go</strong> language"}]}}`))
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(page))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(ProviderBrave, "key")
	client.BaseURL = server.URL
	client.allowPrivate = true

	result, summary, err := client.Run(context.Background(), SearchTool, `{"query":"go"}`)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if summary != "1 result" || !strings.Contains(result, "https://go.dev") {
		t.Fatalf("search = %q, %q", result, summary)
	}
	if strings.Contains(result, "<strong>") || !strings.Contains(result, "The Go language") {
		t.Fatalf("search description = %q", result)
	}

	result, summary, err = client.Run(context.Background(), FetchTool, `{"url":"`+server.URL+`/page"}`)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if summary != "Go & Web" || !strings.HasPrefix(result, "# Go & Web") {
		t.Fatalf("fetch = %q, %q", result, summary)
	}
	if !strings.Contains(result, "The Go language.") {
		t.Fatalf("fetch missing body text: %q", result)
	}
	if strings.Contains(result, "hidden()") || strings.Contains(result, ".x{color:red}") {
		t.Fatalf("fetch kept script/style: %q", result)
	}
	if !strings.HasSuffix(result, "[truncated]") {
		t.Fatalf("fetch not truncated: %q", result[len(result)-30:])
	}

	if _, _, err := client.Run(context.Background(), SearchTool, `{}`); err == nil {
		t.Fatal("empty query: want error")
	}
	if _, _, err := client.Run(context.Background(), FetchTool, `{}`); err == nil {
		t.Fatal("empty url: want error")
	}
}

func TestOllamaProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/web_search":
			if body["query"] != "go" || body["max_results"] != float64(searchResults) {
				t.Errorf("search body = %v", body)
			}
			w.Write([]byte(`{"results":[{"title":"Go","url":"https://go.dev","content":"The Go language"}]}`))
		case "/web_fetch":
			if body["url"] != "https://go.dev" {
				t.Errorf("fetch body = %v", body)
			}
			w.Write([]byte(`{"title":"Go","content":"` + strings.Repeat("a", maxPageRunes+10) + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(ProviderOllama, "key")
	client.BaseURL = server.URL

	result, summary, err := client.Run(context.Background(), SearchTool, `{"query":"go"}`)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if summary != "1 result" || !strings.Contains(result, "https://go.dev") {
		t.Fatalf("search = %q, %q", result, summary)
	}

	result, summary, err = client.Run(context.Background(), FetchTool, `{"url":"https://go.dev"}`)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if summary != "Go" || !strings.HasSuffix(result, "[truncated]") {
		t.Fatalf("fetch summary = %q, suffix = %q", summary, result[len(result)-20:])
	}
}

func TestFetchRejectsLocalURLs(t *testing.T) {
	client := New(ProviderBrave, "key")
	for _, raw := range []string{"http://127.0.0.1/", "http://localhost/", "file:///etc/passwd"} {
		if _, _, err := client.Run(context.Background(), FetchTool, `{"url":"`+raw+`"}`); err == nil {
			t.Errorf("fetch %s: want error", raw)
		}
	}
}

func TestFetchRejectsNonText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF-1.7"))
	}))
	defer server.Close()

	client := New(ProviderBrave, "key")
	client.allowPrivate = true
	if _, _, err := client.Run(context.Background(), FetchTool, `{"url":"`+server.URL+`"}`); err == nil {
		t.Fatal("pdf fetch: want error")
	}
}

func TestFetchRejectsDNSRebinding(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("internal-secret"))
	}))
	defer server.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))

	// The first answer passes the early check; later answers point at loopback.
	calls := 0
	client := New(ProviderBrave, "key")
	client.lookup = func(context.Context, string) ([]netip.Addr, error) {
		calls++
		if calls == 1 {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}

	_, _, err := client.Run(context.Background(), FetchTool, `{"url":"http://rebind.test:`+port+`/"}`)
	if err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("rebinding fetch err = %v, want non-public refusal", err)
	}
	if hit {
		t.Fatal("rebinding fetch reached the loopback server")
	}
}

func TestIsPublicAddr(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8":          true,
		"2606:4700::1111":  true,
		"127.0.0.1":        false,
		"10.1.2.3":         false,
		"100.100.100.200":  false, // CGNAT: Tailscale, Alibaba Cloud metadata
		"169.254.169.254":  false,
		"198.18.0.1":       false,
		"255.255.255.255":  false,
		"::1":              false,
		"fd00::1":          false,
		"64:ff9b::7f00:1":  false, // NAT64 of 127.0.0.1
		"2002:7f00:1::":    false, // 6to4 of 127.0.0.1
		"::ffff:127.0.0.1": false,
		"::ffff:10.0.0.1":  false,
		"::ffff:8.8.8.8":   true,
	}
	for raw, want := range cases {
		if got := isPublicAddr(netip.MustParseAddr(raw).Unmap()); got != want {
			t.Errorf("isPublicAddr(%s) = %v, want %v", raw, got, want)
		}
	}
}

func TestFetchAcceptsMixedCaseContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "Text/HTML; Charset=UTF-8")
		w.Write([]byte("<html><head><title>Mixed</title></head><body><p>case</p></body></html>"))
	}))
	defer server.Close()

	client := New(ProviderBrave, "key")
	client.allowPrivate = true
	result, summary, err := client.Run(context.Background(), FetchTool, `{"url":"`+server.URL+`"}`)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if summary != "Mixed" || !strings.Contains(result, "case") {
		t.Fatalf("fetch = %q, %q", result, summary)
	}
}

func TestFetchCapsTitleAndSearchResults(t *testing.T) {
	longTitle := strings.Repeat("t", 10000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/web/search":
			w.Write([]byte(`{"web":{"results":[` +
				`{"title":"a","url":"https://a"},{"title":"b","url":"https://b"},{"title":"c","url":"https://c"}]}}`))
		default:
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html><head><title>" + longTitle + "</title></head><body>x</body></html>"))
		}
	}))
	defer server.Close()

	client := New(ProviderBrave, "key")
	client.BaseURL = server.URL
	client.allowPrivate = true

	result, summary, err := client.Run(context.Background(), SearchTool, `{"query":"q"}`)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if summary != "2 results" || strings.Contains(result, "https://c") {
		t.Fatalf("search = %q, %q; want 2 results", result, summary)
	}

	_, summary, err = client.Run(context.Background(), FetchTool, `{"url":"`+server.URL+`/page"}`)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if n := len([]rune(summary)); n > maxTitleRunes+1 {
		t.Fatalf("title has %d runes, want at most %d", n, maxTitleRunes+1)
	}
}
