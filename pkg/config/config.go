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
	Model        string `json:"model"`
	SystemPrompt string `json:"systemPrompt"`
}

// Default returns the configuration used on first launch.
func Default() Config {
	return Config{
		BaseURL: "https://api.openai.com/v1",
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
	// Migrate configs written before BaseURL existed (or with a cleared value).
	if cfg.BaseURL == "" {
		cfg.BaseURL = Default().BaseURL
		if err := m.Save(cfg); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

// Save writes the config atomically.
func (m *Manager) Save(cfg Config) error {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.Model = strings.TrimSpace(cfg.Model)

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
