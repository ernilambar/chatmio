package webtools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunSearchAndFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/web_search":
			if body["query"] != "go" {
				t.Errorf("query = %v", body["query"])
			}
			w.Write([]byte(`{"results":[{"title":"Go","url":"https://go.dev","content":"The Go language"}]}`))
		case "/web_fetch":
			w.Write([]byte(`{"title":"Go","content":"` + strings.Repeat("a", maxPageRunes+10) + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New("key")
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

	if _, _, err := client.Run(context.Background(), SearchTool, `{}`); err == nil {
		t.Fatal("empty query: want error")
	}
}
