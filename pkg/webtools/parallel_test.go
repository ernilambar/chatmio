package webtools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseReply wraps a JSON-RPC result in one event stream frame, as the Parallel
// MCP server does.
func sseReply(t *testing.T, text string) string {
	t.Helper()
	reply := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"result": map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
		},
	}
	data, err := json.Marshal(reply)
	if err != nil {
		t.Fatalf("marshal reply: %v", err)
	}
	return "event: message\ndata: " + string(data) + "\n\n"
}

func TestParallelProvider(t *testing.T) {
	var gotAuth, gotTool string
	var gotArgs map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.Method != "tools/call" {
			t.Errorf("method = %q", body.Method)
		}
		gotTool = body.Params.Name
		gotArgs = body.Params.Arguments

		w.Header().Set("Content-Type", "text/event-stream")
		switch body.Params.Name {
		case SearchTool:
			payload := `{"search_id":"s1","results":[
				{"url":"https://go.dev","title":"Go <b>lang</b>","excerpts":["` + strings.Repeat("a", maxResultRunes+50) + `"]},
				{"url":"https://example.com/2","title":"Two","excerpts":["second"]},
				{"url":"https://example.com/3","title":"Three","excerpts":["third"]}]}`
			w.Write([]byte(sseReply(t, payload)))
		case FetchTool:
			w.Write([]byte(sseReply(t, `{"extract_id":"e1","results":[{"url":"https://go.dev","title":"Go","excerpts":["# Release History","go1.27.4"]}]}`)))
		default:
			w.Write([]byte(sseReply(t, "boom")))
		}
	}))
	defer server.Close()

	client := New(ProviderParallel, "")
	client.BaseURL = server.URL

	result, summary, err := client.Run(context.Background(), SearchTool,
		`{"objective":"latest Go release","search_queries":["go release","go 1.27"]}`)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("anonymous search sent Authorization = %q", gotAuth)
	}
	if gotTool != SearchTool {
		t.Errorf("tool = %q", gotTool)
	}
	if gotArgs["objective"] != "latest Go release" {
		t.Errorf("objective = %v", gotArgs["objective"])
	}
	queries, _ := gotArgs["search_queries"].([]any)
	if len(queries) != 2 || queries[0] != "go release" {
		t.Errorf("search_queries = %v", gotArgs["search_queries"])
	}
	if summary != "2 results" || !strings.Contains(result, "https://go.dev") {
		t.Fatalf("search = %q, %q", result, summary)
	}
	if strings.Contains(result, "<b>") || !strings.Contains(result, "Go lang") {
		t.Fatalf("search title not plain text: %q", result)
	}
	if strings.Count(result, "[truncated]") != 1 {
		t.Fatalf("search excerpts not truncated: %q", result)
	}
	if strings.Contains(result, "https://example.com/3") {
		t.Fatalf("search kept more than %d results: %q", searchResults, result)
	}

	result, summary, err = client.Run(context.Background(), FetchTool, `{"url":"https://go.dev"}`)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotTool != FetchTool {
		t.Errorf("tool = %q", gotTool)
	}
	urls, _ := gotArgs["urls"].([]any)
	if len(urls) != 1 || urls[0] != "https://go.dev" {
		t.Errorf("urls = %v", gotArgs["urls"])
	}
	if summary != "Go" || !strings.HasPrefix(result, "# Go\n\n") {
		t.Fatalf("fetch = %q, %q", result, summary)
	}
	if !strings.Contains(result, "go1.27.4") {
		t.Fatalf("fetch missing excerpt: %q", result)
	}

	if _, _, err := client.Run(context.Background(), SearchTool, `{}`); err == nil {
		t.Fatal("empty arguments: want error")
	}
	if _, _, err := client.Run(context.Background(), FetchTool, `{"url":"file:///etc/passwd"}`); err == nil {
		t.Fatal("file url: want error")
	}
}

func TestParallelSearchFallsBackToQuery(t *testing.T) {
	var gotArgs map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Params struct {
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotArgs = body.Params.Arguments
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sseReply(t, `{"results":[{"url":"https://go.dev","title":"Go","excerpts":["ok"]}]}`)))
	}))
	defer server.Close()

	client := New(ProviderParallel, "key")
	client.BaseURL = server.URL

	if _, _, err := client.Run(context.Background(), SearchTool, `{"query":"go release"}`); err != nil {
		t.Fatalf("search: %v", err)
	}
	if gotArgs["objective"] != "go release" {
		t.Errorf("objective = %v", gotArgs["objective"])
	}
	if queries, _ := gotArgs["search_queries"].([]any); len(queries) != 1 {
		t.Errorf("search_queries = %v", gotArgs["search_queries"])
	}
}

func TestParallelToolError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: message\ndata: " +
			`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"Missing required argument: search_queries."}],"isError":true}}` +
			"\n\n"))
	}))
	defer server.Close()

	client := New(ProviderParallel, "")
	client.BaseURL = server.URL

	_, _, err := client.Run(context.Background(), SearchTool, `{"objective":"go","search_queries":["go"]}`)
	if err == nil || !strings.Contains(err.Error(), "search_queries") {
		t.Fatalf("error = %v", err)
	}
}

func TestParallelDefinitions(t *testing.T) {
	defs := New(ProviderParallel, "").Definitions()
	if len(defs) != 2 {
		t.Fatalf("definitions = %d", len(defs))
	}
	search := defs[0].Function
	if search.Name != SearchTool {
		t.Fatalf("first tool = %q", search.Name)
	}
	params, _ := search.Parameters.(map[string]any)
	required, _ := params["required"].([]string)
	if len(required) != 2 || required[0] != "objective" || required[1] != "search_queries" {
		t.Fatalf("required = %v", required)
	}

	brave := New(ProviderBrave, "key").Definitions()
	braveParams, _ := brave[0].Function.Parameters.(map[string]any)
	required, _ = braveParams["required"].([]string)
	if len(required) != 1 || required[0] != "query" {
		t.Fatalf("brave required = %v", required)
	}
}

func TestNormalizeProvider(t *testing.T) {
	cases := map[string]string{
		"":         ProviderParallel,
		"parallel": ProviderParallel,
		"brave":    ProviderBrave,
		"ollama":   ProviderOllama,
		"bogus":    ProviderParallel,
	}
	for in, want := range cases {
		if got := NormalizeProvider(in); got != want {
			t.Errorf("NormalizeProvider(%q) = %q, want %q", in, got, want)
		}
	}
}
