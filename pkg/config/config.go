// Package config manages the local JSON configuration file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Theme identifies the UI colour scheme.
type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

// Provider is an OpenAI-compatible endpoint preset.
type Provider struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	BaseURL string            `json:"baseUrl"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Config is the persisted application configuration.
type Config struct {
	Theme          Theme      `json:"theme"`
	ActiveProvider string     `json:"activeProvider"`
	ActiveModel    string     `json:"activeModel"`
	SystemPrompt   string     `json:"systemPrompt"`
	Providers      []Provider `json:"providers"`
}

// Default returns the configuration used on first launch.
func Default() Config {
	return Config{
		Theme:     ThemeSystem,
		Providers: []Provider{},
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

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config: parse %s: %w", m.path, err)
	}
	if cfg.Providers == nil {
		cfg.Providers = []Provider{}
	}
	return cfg, nil
}

// Save writes the config atomically.
func (m *Manager) Save(cfg Config) error {
	if cfg.Theme == "" {
		cfg.Theme = ThemeSystem
	}
	switch cfg.Theme {
	case ThemeSystem, ThemeLight, ThemeDark:
	default:
		return fmt.Errorf("config: invalid theme %q", cfg.Theme)
	}
	if cfg.Providers == nil {
		cfg.Providers = []Provider{}
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
