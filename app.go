package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/ernilambar/chattu/pkg/ai"
	"github.com/ernilambar/chattu/pkg/config"
	"github.com/ernilambar/chattu/pkg/db"
)

const (
	defaultThreadTitle = "New chat"
	defaultTemperature = 0.7
	defaultTopP        = 1.0
	defaultMaxTokens   = 2048
	titleTemperature   = 0.2
	titleMaxTokens     = 32
	titlePrompt        = "Generate a short title for this conversation. Reply with only the title: 3 to 6 words, no quotes, no trailing punctuation."
)

// App is the backend bridge exposed to the frontend.
type App struct {
	ctx     context.Context
	dataDir string
	config  *config.Manager
	store   *db.Store
	sendMu  sync.Mutex
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
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return errors.New("base URL is required")
	}
	if strings.TrimSpace(cfg.ChatModel) == "" {
		return errors.New("chat model is required")
	}
	return a.config.Save(cfg)
}

// ListThreads returns all chat threads, most recently updated first.
func (a *App) ListThreads() ([]db.Thread, error) {
	if a.store == nil {
		return nil, errors.New("database unavailable")
	}
	return a.store.ListThreads()
}

// CreateThread creates an empty chat thread and returns it.
func (a *App) CreateThread() (db.Thread, error) {
	if a.store == nil {
		return db.Thread{}, errors.New("database unavailable")
	}
	now := time.Now().UnixMilli()
	thread := db.Thread{
		ID:               uuid.NewString(),
		Title:            defaultThreadTitle,
		Temperature:      defaultTemperature,
		TopP:             defaultTopP,
		MaxTokens:        defaultMaxTokens,
		FrequencyPenalty: 0,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := a.store.CreateThread(thread); err != nil {
		return db.Thread{}, err
	}
	return thread, nil
}

// GetMessages returns the messages of a thread in chronological order.
func (a *App) GetMessages(threadID string) ([]db.Message, error) {
	if a.store == nil {
		return nil, errors.New("database unavailable")
	}
	return a.store.ListMessages(threadID)
}

// SendMessage stores the user message, streams the assistant reply via
// chat:delta events and persists the final message. It returns when streaming
// finishes.
func (a *App) SendMessage(threadID, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("message is empty")
	}
	if a.ctx == nil {
		return errors.New("app is not ready")
	}
	if a.store == nil {
		return errors.New("database unavailable")
	}

	a.sendMu.Lock()
	defer a.sendMu.Unlock()

	thread, err := a.store.GetThread(threadID)
	if err != nil {
		return err
	}

	cfg, err := a.config.Load()
	if err != nil {
		return err
	}
	if cfg.BaseURL == "" {
		return errors.New("set a base URL in settings")
	}
	if cfg.ChatModel == "" {
		return errors.New("set a chat model in settings")
	}

	now := time.Now().UnixMilli()
	userMessage := db.Message{
		ID:        uuid.NewString(),
		ThreadID:  threadID,
		Role:      string(ai.RoleUser),
		Content:   content,
		CreatedAt: now,
	}
	if err := a.store.AddMessage(userMessage); err != nil {
		return err
	}

	history, err := a.store.ListMessages(threadID)
	if err != nil {
		return err
	}
	prompt := make([]ai.Message, 0, len(history)+1)
	systemPrompt := thread.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = cfg.SystemPrompt
	}
	if systemPrompt != "" {
		prompt = append(prompt, ai.Message{Role: ai.RoleSystem, Content: systemPrompt})
	}
	for _, m := range history {
		prompt = append(prompt, ai.Message{Role: ai.Role(m.Role), Content: m.Content})
	}

	needsTitle := thread.Title == "" || thread.Title == defaultThreadTitle

	client := ai.New(cfg.BaseURL, cfg.APIKey, nil)
	var reply strings.Builder
	err = client.StreamChat(a.ctx, ai.ChatOptions{
		Model:            cfg.ChatModel,
		Messages:         prompt,
		Temperature:      &thread.Temperature,
		TopP:             &thread.TopP,
		MaxTokens:        &thread.MaxTokens,
		FrequencyPenalty: &thread.FrequencyPenalty,
	}, func(token string) {
		reply.WriteString(token)
		runtime.EventsEmit(a.ctx, "chat:delta", chatDelta{
			ThreadID: threadID,
			Delta:    token,
		})
	})
	if err != nil {
		return err
	}

	assistantMessage := db.Message{
		ID:        uuid.NewString(),
		ThreadID:  threadID,
		Role:      string(ai.RoleAssistant),
		Content:   reply.String(),
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := a.store.AddMessage(assistantMessage); err != nil {
		return err
	}
	title := thread.Title
	if needsTitle {
		title = a.generateTitle(client, cfg, content)
	}
	if err := a.store.UpdateThreadMeta(threadID, title, time.Now().UnixMilli()); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "chat:done", chatDone{
		ThreadID: threadID,
		Message:  assistantMessage,
	})
	return nil
}

type chatDelta struct {
	ThreadID string `json:"threadId"`
	Delta    string `json:"delta"`
}

type chatDone struct {
	ThreadID string     `json:"threadId"`
	Message  db.Message `json:"message"`
}

// generateTitle asks the title model for a short thread title and falls back to
// the first user message when the request fails.
func (a *App) generateTitle(client *ai.Client, cfg config.Config, content string) string {
	model := cfg.TitleModel
	if model == "" {
		model = cfg.ChatModel
	}
	if model == "" {
		return titleFromContent(content)
	}

	temperature := titleTemperature
	maxTokens := titleMaxTokens
	var generated strings.Builder
	err := client.StreamChat(a.ctx, ai.ChatOptions{
		Model: model,
		Messages: []ai.Message{
			{Role: ai.RoleSystem, Content: titlePrompt},
			{Role: ai.RoleUser, Content: content},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
	}, func(token string) {
		generated.WriteString(token)
	})
	if err != nil {
		log.Printf("app: generate title: %v", err)
		return titleFromContent(content)
	}
	return normalizeTitle(generated.String(), content)
}

func normalizeTitle(raw, content string) string {
	title := strings.TrimSpace(raw)
	title = strings.TrimSpace(strings.TrimPrefix(title, "Title:"))
	title = strings.TrimSpace(strings.Trim(title, "\"'`*"))
	title = strings.Join(strings.Fields(title), " ")
	title = strings.TrimRight(title, ".!?,;:")
	if title == "" {
		return titleFromContent(content)
	}
	return truncateRunes(title, 60)
}

func titleFromContent(content string) string {
	title := strings.Join(strings.Fields(content), " ")
	if title == "" {
		return defaultThreadTitle
	}
	return truncateRunes(title, 60)
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return s
}
