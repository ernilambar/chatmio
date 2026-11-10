package config

import (
	"os"
	"path/filepath"
	"testing"
)

func load(t *testing.T, body string) Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := NewManager(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestLoadDisablesWebToolsByDefault(t *testing.T) {
	cfg := load(t, `{"baseUrl":"https://example.com"}`)
	if cfg.WebToolsEnabled {
		t.Fatal("WebToolsEnabled = true, want the default false")
	}
	if cfg.SearchProvider != "brave" {
		t.Fatalf("SearchProvider = %q, want brave", cfg.SearchProvider)
	}
}

func TestLoadKeepsExplicitWebToolsOn(t *testing.T) {
	cfg := load(t, `{"webToolsEnabled":true,"searchProvider":"ollama"}`)
	if !cfg.WebToolsEnabled {
		t.Fatal("WebToolsEnabled = false, want explicit true")
	}
	if cfg.SearchProvider != "ollama" {
		t.Fatalf("SearchProvider = %q, want ollama", cfg.SearchProvider)
	}
}
