// Package config manages the local JSON configuration file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is the persisted application configuration.
// APIKey is stored in plain text for now; OS keychain support comes later.
type Config struct {
	BaseURL      string `json:"baseUrl"`
	APIKey       string `json:"apiKey"`
	ChatModel    string `json:"chatModel"`
	TitleModel   string `json:"titleModel"`
	SystemPrompt string `json:"systemPrompt"`
	// ContextWindow is the default model context size in tokens used for the
	// context warning. Zero disables the warning unless a chat sets its own.
	ContextWindow int `json:"contextWindow"`
	// Generation defaults are copied into new chats. Nil means the provider
	// default and leaves the field unset in the request.
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"topP,omitempty"`
	MaxTokens        *int     `json:"maxTokens,omitempty"`
	FrequencyPenalty *float64 `json:"frequencyPenalty,omitempty"`
	// WebToolsEnabled is the global consent switch for web search and page
	// fetch. Turning it on starts new chats with web tools enabled; each chat
	// still has its own toggle to turn them off.
	WebToolsEnabled bool `json:"webToolsEnabled"`
	// SearchProvider selects the web search backend: "parallel", "brave", or
	// "ollama". Empty means "parallel".
	SearchProvider string `json:"searchProvider"`
	// ParallelAPIKey is optional. Without one, Parallel runs on its free
	// anonymous MCP tier at lower rate limits.
	ParallelAPIKey string `json:"parallelApiKey"`
	// BraveAPIKey enables the Brave Search backend. Empty disables it.
	BraveAPIKey string `json:"braveApiKey"`
	// OllamaAPIKey enables the Ollama backend. Empty disables it.
	OllamaAPIKey string `json:"ollamaApiKey"`
	// Theme is the UI color theme: "system", "light", or "dark". Empty is
	// normalized to "system".
	Theme string `json:"theme"`
}

// Default returns the configuration used on first launch.
func Default() Config {
	return Config{
		BaseURL: "https://api.openai.com/v1",
		// Web tools stay off until the user opts in, so no search query leaves
		// the machine by default.
		WebToolsEnabled: false,
		SearchProvider:  "parallel",
		Theme:           "system",
	}
}

// Manager loads and stores a Config at a fixed path.
type Manager struct {
	path string
}

// NewManager returns a Manager for the config file at path.
func NewManager(path string) *Manager {
	return &Manager{path: path}
}

// Path returns the config file path.
func (m *Manager) Path() string {
	return m.path
}

// Load reads the config file. A missing file is created with defaults.
func (m *Manager) Load() (Config, error) {
	data, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		cfg := Default()
		if err := m.Save(cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("config: read %s: %w", m.path, err)
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config: parse %s: %w", m.path, err)
	}
	return cfg, nil
}

// Save writes the config atomically.
func (m *Manager) Save(cfg Config) error {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.ChatModel = strings.TrimSpace(cfg.ChatModel)
	cfg.TitleModel = strings.TrimSpace(cfg.TitleModel)
	cfg.BraveAPIKey = strings.TrimSpace(cfg.BraveAPIKey)
	cfg.OllamaAPIKey = strings.TrimSpace(cfg.OllamaAPIKey)
	cfg.ParallelAPIKey = strings.TrimSpace(cfg.ParallelAPIKey)
	switch cfg.Theme {
	case "", "system":
		cfg.Theme = "system"
	case "light", "dark":
	default:
		return fmt.Errorf("config: invalid theme %q", cfg.Theme)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("config: encode: %w", err)
	}

	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: create dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return fmt.Errorf("config: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("config: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: close: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("config: chmod: %w", err)
	}
	if err := os.Rename(tmpName, m.path); err != nil {
		return fmt.Errorf("config: replace: %w", err)
	}
	return nil
}
