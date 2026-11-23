package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ernilambar/chatmio/pkg/config"
	"github.com/ernilambar/chatmio/pkg/db"
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

// newSeededApp returns an App wired to a temporary config file and stores, so
// thread creation can be exercised without touching the user's data.
func newSeededApp(t *testing.T, cfg config.Config) *App {
	t.Helper()
	dir := t.TempDir()
	manager := config.NewManager(filepath.Join(dir, "config.json"))
	if err := manager.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store, err := db.Open(filepath.Join(dir, "chats.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	tempStore, err := db.OpenMemory()
	if err != nil {
		store.Close()
		t.Fatalf("open temporary database: %v", err)
	}
	t.Cleanup(func() {
		store.Close()
		tempStore.Close()
	})
	return &App{
		dataDir:   dir,
		config:    manager,
		store:     store,
		tempStore: tempStore,
		tempIDs:   make(map[string]bool),
	}
}

func TestCreateThreadSeedsWebTools(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want bool
	}{
		{
			name: "default config leaves web tools off",
			cfg:  config.Default(),
			want: false,
		},
		{
			name: "enabled with parallel needs no key",
			cfg:  config.Config{WebToolsEnabled: true, SearchProvider: "parallel"},
			want: true,
		},
		{
			name: "enabled with brave key",
			cfg:  config.Config{WebToolsEnabled: true, SearchProvider: "brave", BraveAPIKey: "k"},
			want: true,
		},
		{
			name: "enabled with brave but no key",
			cfg:  config.Config{WebToolsEnabled: true, SearchProvider: "brave"},
			want: false,
		},
		{
			name: "enabled with ollama key",
			cfg:  config.Config{WebToolsEnabled: true, SearchProvider: "ollama", OllamaAPIKey: "k"},
			want: true,
		},
		{
			name: "enabled with ollama but no key",
			cfg:  config.Config{WebToolsEnabled: true, SearchProvider: "ollama"},
			want: false,
		},
		{
			name: "disabled overrides a present key",
			cfg:  config.Config{WebToolsEnabled: false, SearchProvider: "brave", BraveAPIKey: "k"},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newSeededApp(t, tc.cfg)
			thread, err := app.CreateThread()
			if err != nil {
				t.Fatalf("CreateThread: %v", err)
			}
			if thread.WebTools != tc.want {
				t.Fatalf("returned WebTools = %v, want %v", thread.WebTools, tc.want)
			}

			threads, err := app.ListThreads()
			if err != nil {
				t.Fatalf("ListThreads: %v", err)
			}
			if len(threads) != 1 {
				t.Fatalf("len(threads) = %d, want 1", len(threads))
			}
			if threads[0].WebTools != tc.want {
				t.Fatalf("stored WebTools = %v, want %v", threads[0].WebTools, tc.want)
			}
		})
	}
}

func TestCreateTemporaryThreadSeedsWebTools(t *testing.T) {
	app := newSeededApp(t, config.Config{WebToolsEnabled: true, SearchProvider: "parallel"})
	thread, err := app.CreateTemporaryThread()
	if err != nil {
		t.Fatalf("CreateTemporaryThread: %v", err)
	}
	if !thread.WebTools {
		t.Fatal("WebTools = false, want true for an enabled parallel config")
	}
}

func TestCreateThreadKeepsExistingThreadsUnchanged(t *testing.T) {
	app := newSeededApp(t, config.Default())
	first, err := app.CreateThread()
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if first.WebTools {
		t.Fatal("WebTools = true, want false with web tools disabled")
	}

	cfg, err := app.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	cfg.WebToolsEnabled = true
	cfg.SearchProvider = "parallel"
	if err := app.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	second, err := app.CreateThread()
	if err != nil {
		t.Fatalf("CreateThread after enabling: %v", err)
	}
	if !second.WebTools {
		t.Fatal("new thread WebTools = false, want true after enabling web tools")
	}

	threads, err := app.ListThreads()
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	for _, thread := range threads {
		if thread.ID == first.ID && thread.WebTools {
			t.Fatal("existing thread was retroactively enabled, want it left off")
		}
	}
}
