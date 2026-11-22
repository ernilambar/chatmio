package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTestConnectionReportsModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path = %q, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("authorization = %q", got)
		}
		fmt.Fprint(w, `{"data":[{"id":"model-a"},{"id":"model-b"}]}`)
	}))
	defer server.Close()

	app := &App{}
	got, err := app.TestConnection(server.URL, "sk-test", "model-a")
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if !strings.Contains(got, "model-a is available") {
		t.Fatalf("message = %q, want model-a available", got)
	}

	got, err = app.TestConnection(server.URL, "sk-test", "model-z")
	if err != nil {
		t.Fatalf("TestConnection unknown model: %v", err)
	}
	if !strings.Contains(got, "does not list model-z") {
		t.Fatalf("message = %q, want unknown model note", got)
	}
}

func TestOpenCodeSessionHeaders(t *testing.T) {
	const session = "123e4567-e89b-12d3-a456-426614174000"
	for _, baseURL := range []string{
		"https://opencode.ai/inference/go/v1",
		"https://opencode.ai/zen/go/v1",
		"https://opencode.ai/zen/v1",
		"https://console.opencode.ai/v1",
	} {
		if got := openCodeSessionHeaders(baseURL, session)["x-opencode-session"]; got != session {
			t.Errorf("%s: session = %q, want %q", baseURL, got, session)
		}
	}
	for _, baseURL := range []string{
		"https://api.openai.com/v1",
		"https://notopencode.ai/v1",
		"https://opencode.ai.evil.test/v1",
		"",
	} {
		if got := openCodeSessionHeaders(baseURL, session); got != nil {
			t.Errorf("%s: headers = %v, want nil", baseURL, got)
		}
	}
	if got := openCodeSessionHeaders("https://opencode.ai/inference/go/v1", ""); got != nil {
		t.Errorf("headers = %v, want nil for empty session", got)
	}
}

func TestTestConnectionReportsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"Incorrect API key provided"}}`)
	}))
	defer server.Close()

	_, err := (&App{}).TestConnection(server.URL, "bad", "model-a")
	if err == nil || !strings.Contains(err.Error(), "Incorrect API key provided") {
		t.Fatalf("err = %v, want provider message", err)
	}
}

func TestTestConnectionFallsBackToCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			http.NotFound(w, r)
		case "/chat/completions":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
			}
			if body["model"] != "model-a" {
				t.Errorf("model = %v, want model-a", body["model"])
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n")
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	got, err := (&App{}).TestConnection(server.URL+"/", "key", "model-a")
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if got != "Connected." {
		t.Fatalf("message = %q, want Connected.", got)
	}
}
