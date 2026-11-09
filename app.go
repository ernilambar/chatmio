package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/ernilambar/chattu/pkg/config"
	"github.com/ernilambar/chattu/pkg/db"
	"github.com/ernilambar/chattu/pkg/keychain"
)

// App is the backend bridge exposed to the frontend.
type App struct {
	ctx     context.Context
	dataDir string
	config  *config.Manager
	store   *db.Store
}

// NewApp builds the application services.
func NewApp() *App {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		log.Printf("app: resolve config dir: %v", err)
		baseDir = "."
	}
	dataDir := filepath.Join(baseDir, "chattu")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		log.Printf("app: create data dir: %v", err)
	}

	return &App{
		dataDir: dataDir,
		config:  config.NewManager(filepath.Join(dataDir, "config.json")),
	}
}

// startup is called by Wails once the window is ready.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	if _, err := a.config.Load(); err != nil {
		log.Printf("app: load config: %v", err)
	}

	store, err := db.Open(filepath.Join(a.dataDir, "chats.db"))
	if err != nil {
		log.Printf("app: open database: %v", err)
		return
	}
	a.store = store
}

// shutdown is called by Wails before the application exits.
func (a *App) shutdown(context.Context) {
	if err := a.store.Close(); err != nil {
		log.Printf("app: close database: %v", err)
	}
}

// GetConfig returns the persisted configuration.
func (a *App) GetConfig() (config.Config, error) {
	return a.config.Load()
}

// SaveConfig persists the configuration.
func (a *App) SaveConfig(cfg config.Config) error {
	return a.config.Save(cfg)
}

// SetAPIKey stores a provider API key in the OS keychain.
func (a *App) SetAPIKey(providerID, apiKey string) error {
	if providerID == "" {
		return errors.New("provider id is required")
	}
	return keychain.Set(providerID, apiKey)
}

// DeleteAPIKey removes a provider API key from the OS keychain.
func (a *App) DeleteAPIKey(providerID string) error {
	return keychain.Delete(providerID)
}

// HasAPIKey reports whether a provider has a stored API key.
func (a *App) HasAPIKey(providerID string) (bool, error) {
	return keychain.Has(providerID)
}
