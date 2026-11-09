package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/ernilambar/chattu/pkg/ai"
	"github.com/ernilambar/chattu/pkg/config"
	"github.com/ernilambar/chattu/pkg/db"
)

const (
	defaultThreadTitle     = "New chat"
	defaultTemperature     = 0.7
	defaultTopP            = 1.0
	defaultMaxTokens       = 2048
	titleTemperature       = 0.2
	titleMaxTokens         = 512
	titleTimeout           = 30 * time.Second
	titlePrompt            = "Generate a short title for this conversation. Reply with only the title: 3 to 6 words, no quotes, no trailing punctuation."
	maxAttachments         = 10
	maxAttachmentBytes     = 10 << 20
	maxAttachmentsBytes    = 20 << 20
	maxTextAttachmentBytes = 200 << 10
	maxTitleSourceRunes    = 4000
)

// App is the backend bridge exposed to the frontend.
type App struct {
	ctx     context.Context
	dataDir string
	config  *config.Manager
	store   *db.Store
	sendMu  sync.Mutex

	// streamCancel stops the reply currently streaming, if any.
	streamMu     sync.Mutex
	streamCancel context.CancelFunc

	// Title requests run after the reply is delivered so they never delay it.
	titleMu sync.Mutex
	titling map[string]bool
	closed  bool
	titleWg sync.WaitGroup
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
		titling: make(map[string]bool),
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
	a.titleMu.Lock()
	a.closed = true
	a.titleMu.Unlock()
	a.titleWg.Wait()
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

// SearchThreads finds threads by title or message content, most recently
// updated first.
func (a *App) SearchThreads(query string) ([]db.SearchResult, error) {
	if a.store == nil {
		return nil, errors.New("database unavailable")
	}
	return a.store.SearchThreads(query, 20)
}

// CreateThread creates an empty chat thread and returns it.
func (a *App) CreateThread() (db.Thread, error) {
	if a.store == nil {
		return db.Thread{}, errors.New("database unavailable")
	}
	cfg, err := a.config.Load()
	if err != nil {
		return db.Thread{}, err
	}
	now := time.Now().UnixMilli()
	thread := db.Thread{
		ID:               uuid.NewString(),
		Title:            defaultThreadTitle,
		Model:            strings.TrimSpace(cfg.ChatModel),
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
	messages, err := a.store.ListMessages(threadID)
	if err != nil {
		return nil, err
	}
	for i := range messages {
		for j := range messages[i].Attachments {
			att := &messages[i].Attachments[j]
			if isImageAttachment(att.MimeType) {
				att.DataURL = attachmentDataURL(*att)
			}
		}
	}
	return messages, nil
}

// SetThreadModel changes the model used by one thread.
func (a *App) SetThreadModel(threadID, model string) error {
	if a.store == nil {
		return errors.New("database unavailable")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("model is required")
	}
	return a.store.SetThreadModel(threadID, model)
}

// RenameThread sets a user-provided title.
func (a *App) RenameThread(threadID, title string) error {
	if a.store == nil {
		return errors.New("database unavailable")
	}
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		return errors.New("title is required")
	}
	return a.store.UpdateThreadMeta(threadID, truncateRunes(title, 120), time.Now().UnixMilli())
}

// DeleteThread removes a thread and its messages.
func (a *App) DeleteThread(threadID string) error {
	if a.store == nil {
		return errors.New("database unavailable")
	}
	return a.store.DeleteThread(threadID)
}

// ListModels fetches available model identifiers for the provider settings the
// user is editing, even before they are saved.
func (a *App) ListModels(baseURL, apiKey string) ([]string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, errors.New("base URL is required")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return ai.New(baseURL, apiKey, nil).ListModels(ctx)
}

// SendMessage stores the user message, streams the assistant reply via
// chat:delta events and persists the final message. It returns when streaming
// finishes.
func (a *App) SendMessage(threadID, content string, attachments []db.AttachmentInput) error {
	content = strings.TrimSpace(content)
	if content == "" && len(attachments) == 0 {
		return errors.New("message is empty")
	}

	a.sendMu.Lock()
	defer a.sendMu.Unlock()

	thread, cfg, model, err := a.prepareReply(threadID)
	if err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	files, err := prepareAttachments(attachments, now)
	if err != nil {
		return err
	}
	userMessage := db.Message{
		ID:          uuid.NewString(),
		ThreadID:    threadID,
		Role:        string(ai.RoleUser),
		Content:     content,
		Attachments: files,
		CreatedAt:   now,
	}
	if err := a.store.AddMessage(userMessage); err != nil {
		return err
	}

	return a.streamReply(thread, cfg, model)
}

// CancelReply stops the reply that is currently streaming. Text received so
// far is kept and the calling SendMessage/RegenerateReply/EditMessage returns
// without an error.
func (a *App) CancelReply() {
	a.streamMu.Lock()
	cancel := a.streamCancel
	a.streamMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *App) setStreamCancel(cancel context.CancelFunc) {
	a.streamMu.Lock()
	a.streamCancel = cancel
	a.streamMu.Unlock()
}

func (a *App) clearStreamCancel() {
	a.streamMu.Lock()
	a.streamCancel = nil
	a.streamMu.Unlock()
}

// RegenerateReply discards the trailing assistant reply, if any, and streams a
// new reply for the last user message. It also retries a send that failed
// before any assistant reply was stored.
func (a *App) RegenerateReply(threadID string) error {
	a.sendMu.Lock()
	defer a.sendMu.Unlock()

	thread, cfg, model, err := a.prepareReply(threadID)
	if err != nil {
		return err
	}

	messages, err := a.store.ListMessages(threadID)
	if err != nil {
		return err
	}
	lastUser := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == string(ai.RoleUser) {
			lastUser = i
			break
		}
	}
	if lastUser < 0 {
		return errors.New("no user message to regenerate")
	}
	if lastUser < len(messages)-1 {
		if err := a.store.DeleteMessagesAfter(threadID, messages[lastUser].ID); err != nil {
			return err
		}
	}

	return a.streamReply(thread, cfg, model)
}

// EditMessage replaces a user message's text, discards that message's reply and
// everything after it, then streams a fresh reply. Attachments are preserved.
func (a *App) EditMessage(threadID, messageID, content string) error {
	content = strings.TrimSpace(content)

	a.sendMu.Lock()
	defer a.sendMu.Unlock()

	thread, cfg, model, err := a.prepareReply(threadID)
	if err != nil {
		return err
	}

	messages, err := a.store.ListMessages(threadID)
	if err != nil {
		return err
	}
	target := -1
	for i := range messages {
		if messages[i].ID == messageID {
			target = i
			break
		}
	}
	if target < 0 {
		return errors.New("message not found")
	}
	if messages[target].Role != string(ai.RoleUser) {
		return errors.New("only user messages can be edited")
	}
	if content == "" && len(messages[target].Attachments) == 0 {
		return errors.New("message is empty")
	}

	if err := a.store.UpdateMessageContent(messageID, content); err != nil {
		return err
	}
	if target < len(messages)-1 {
		if err := a.store.DeleteMessagesAfter(threadID, messageID); err != nil {
			return err
		}
	}

	return a.streamReply(thread, cfg, model)
}

// prepareReply validates the app, thread and provider settings before a
// streaming request.
func (a *App) prepareReply(threadID string) (db.Thread, config.Config, string, error) {
	if a.ctx == nil {
		return db.Thread{}, config.Config{}, "", errors.New("app is not ready")
	}
	if a.store == nil {
		return db.Thread{}, config.Config{}, "", errors.New("database unavailable")
	}

	thread, err := a.store.GetThread(threadID)
	if err != nil {
		return db.Thread{}, config.Config{}, "", err
	}

	cfg, err := a.config.Load()
	if err != nil {
		return db.Thread{}, config.Config{}, "", err
	}
	if cfg.BaseURL == "" {
		return db.Thread{}, config.Config{}, "", errors.New("set a base URL in settings")
	}
	model := strings.TrimSpace(thread.Model)
	if model == "" {
		model = strings.TrimSpace(cfg.ChatModel)
	}
	if model == "" {
		return db.Thread{}, config.Config{}, "", errors.New("set a chat model in settings")
	}
	return thread, cfg, model, nil
}

// streamReply sends the thread's history to the model, persists the assistant
// message, emits chat:done and then starts the title request.
func (a *App) streamReply(thread db.Thread, cfg config.Config, model string) error {
	history, err := a.store.ListMessages(thread.ID)
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
		prompt = append(prompt, toAIMessage(m))
	}

	client := ai.New(cfg.BaseURL, cfg.APIKey, nil)
	streamCtx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	a.setStreamCancel(cancel)
	defer a.clearStreamCancel()

	var reply strings.Builder
	err = client.StreamChat(streamCtx, ai.ChatOptions{
		Model:            model,
		Messages:         prompt,
		Temperature:      &thread.Temperature,
		TopP:             &thread.TopP,
		MaxTokens:        &thread.MaxTokens,
		FrequencyPenalty: &thread.FrequencyPenalty,
	}, func(token string) {
		reply.WriteString(token)
		runtime.EventsEmit(a.ctx, "chat:delta", chatDelta{
			ThreadID: thread.ID,
			Delta:    token,
		})
	})

	// A user cancel keeps whatever text arrived. Any other error, including
	// cancellation during shutdown, is reported to the caller.
	stopped := false
	if err != nil {
		if errors.Is(streamCtx.Err(), context.Canceled) && a.ctx.Err() == nil {
			stopped = true
		} else {
			return err
		}
	}
	if stopped && strings.TrimSpace(reply.String()) == "" {
		return nil
	}

	assistantMessage := db.Message{
		ID:        uuid.NewString(),
		ThreadID:  thread.ID,
		Role:      string(ai.RoleAssistant),
		Content:   reply.String(),
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := a.store.AddMessage(assistantMessage); err != nil {
		return err
	}
	if err := a.store.TouchThread(thread.ID, time.Now().UnixMilli()); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "chat:done", chatDone{
		ThreadID: thread.ID,
		Message:  assistantMessage,
	})

	if !stopped && (thread.Title == "" || thread.Title == defaultThreadTitle) {
		a.queueTitle(thread, cfg, model, lastUserTitleSource(history))
	}
	return nil
}

// lastUserTitleSource describes the most recent user message for title
// generation.
func lastUserTitleSource(messages []db.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == string(ai.RoleUser) {
			return titleSource(messages[i].Content, messages[i].Attachments)
		}
	}
	return ""
}

// queueTitle requests a title after the reply is delivered, then announces it
// through chat:title. At most one request runs per thread, and a user rename
// always wins over the generated title.
func (a *App) queueTitle(thread db.Thread, cfg config.Config, model, source string) {
	if strings.TrimSpace(source) == "" {
		return
	}

	a.titleMu.Lock()
	if a.closed {
		a.titleMu.Unlock()
		return
	}
	if a.titling == nil {
		a.titling = make(map[string]bool)
	}
	if a.titling[thread.ID] {
		a.titleMu.Unlock()
		return
	}
	a.titling[thread.ID] = true
	a.titleWg.Add(1)
	a.titleMu.Unlock()

	go func() {
		defer a.titleWg.Done()
		defer func() {
			a.titleMu.Lock()
			delete(a.titling, thread.ID)
			a.titleMu.Unlock()
		}()

		client := ai.New(cfg.BaseURL, cfg.APIKey, nil)
		title := a.generateTitle(client, cfg, model, source)
		changed, err := a.store.SetThreadTitleIfUnchanged(thread.ID, thread.Title, title)
		if err != nil {
			log.Printf("app: set generated title: %v", err)
			return
		}
		if !changed {
			return
		}
		runtime.EventsEmit(a.ctx, "chat:title", chatTitle{
			ThreadID: thread.ID,
			Title:    title,
		})
	}()
}

type chatDelta struct {
	ThreadID string `json:"threadId"`
	Delta    string `json:"delta"`
}

type chatDone struct {
	ThreadID string     `json:"threadId"`
	Message  db.Message `json:"message"`
}

type chatTitle struct {
	ThreadID string `json:"threadId"`
	Title    string `json:"title"`
}

func normalizeMIME(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isImageAttachment(mimeType string) bool {
	return strings.HasPrefix(normalizeMIME(mimeType), "image/")
}

func isTextAttachment(mimeType, name string) bool {
	mediaType := normalizeMIME(mimeType)
	mediaType, _, _ = strings.Cut(mediaType, ";")
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json", "application/xml", "application/yaml",
		"application/x-yaml", "application/toml", "application/javascript",
		"application/x-javascript", "application/x-sh", "application/sql":
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".txt", ".json", ".jsonl", ".csv", ".tsv",
		".yaml", ".yml", ".toml", ".xml", ".html", ".htm", ".css", ".js",
		".mjs", ".cjs", ".jsx", ".ts", ".tsx", ".go", ".py", ".rb", ".java",
		".c", ".h", ".cpp", ".hpp", ".rs", ".sh", ".bash", ".zsh", ".sql",
		".log", ".ini", ".conf", ".env":
		return true
	}
	return false
}

func attachmentDataURL(att db.Attachment) string {
	return "data:" + att.MimeType + ";base64," + base64.StdEncoding.EncodeToString(att.Data)
}

func prepareAttachments(inputs []db.AttachmentInput, now int64) ([]db.Attachment, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if len(inputs) > maxAttachments {
		return nil, fmt.Errorf("too many attachments (max %d)", maxAttachments)
	}

	total := 0
	attachments := make([]db.Attachment, 0, len(inputs))
	for _, input := range inputs {
		name := truncateRunes(strings.TrimSpace(input.Name), 200)
		if name == "" {
			name = "attachment"
		}
		encoded := strings.TrimSpace(input.Data)
		if strings.HasPrefix(encoded, "data:") {
			if _, rest, ok := strings.Cut(encoded, ","); ok {
				encoded = rest
			}
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			data, err = base64.RawStdEncoding.DecodeString(encoded)
			if err != nil {
				return nil, fmt.Errorf("attachment %q is not valid base64", name)
			}
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("attachment %q is empty", name)
		}
		if len(data) > maxAttachmentBytes {
			return nil, fmt.Errorf("attachment %q exceeds %d MB", name, maxAttachmentBytes>>20)
		}
		total += len(data)
		if total > maxAttachmentsBytes {
			return nil, fmt.Errorf("attachments exceed %d MB total", maxAttachmentsBytes>>20)
		}

		mimeType := normalizeMIME(input.MimeType)
		if mimeType == "" || mimeType == "application/octet-stream" {
			if inferred := normalizeMIME(mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))); inferred != "" {
				mimeType = inferred
			}
		}
		if mimeType == "" || mimeType == "application/octet-stream" {
			mimeType = normalizeMIME(http.DetectContentType(data))
		}
		if !isImageAttachment(mimeType) && !isTextAttachment(mimeType, name) {
			return nil, fmt.Errorf("attachment %q has unsupported type %s; attach images or text files", name, mimeType)
		}

		attachments = append(attachments, db.Attachment{
			ID:        uuid.NewString(),
			Name:      name,
			MimeType:  mimeType,
			Size:      int64(len(data)),
			Data:      data,
			CreatedAt: now,
		})
	}
	return attachments, nil
}

func titleSource(content string, attachments []db.Attachment) string {
	if content = strings.TrimSpace(content); content != "" {
		return truncateRunes(content, maxTitleSourceRunes)
	}
	if len(attachments) == 0 {
		return ""
	}
	names := make([]string, 0, len(attachments))
	for _, att := range attachments {
		names = append(names, att.Name)
	}
	return "Attached: " + strings.Join(names, ", ")
}

// toAIMessage adds text-file contents inline and turns images into content
// parts. Other attachments are rejected before they reach storage.
func toAIMessage(m db.Message) ai.Message {
	role := ai.Role(m.Role)
	if len(m.Attachments) == 0 {
		return ai.Message{Role: role, Content: m.Content}
	}

	text := strings.TrimSpace(m.Content)
	parts := make([]ai.ContentPart, 0, len(m.Attachments)+1)
	for _, att := range m.Attachments {
		if isImageAttachment(att.MimeType) {
			parts = append(parts, ai.ImagePart(attachmentDataURL(att)))
			continue
		}
		if isTextAttachment(att.MimeType, att.Name) {
			content := string(att.Data)
			if len([]rune(content)) > maxTextAttachmentBytes {
				content = truncateRunes(content, maxTextAttachmentBytes) + "\n[truncated]"
			}
			if text != "" {
				text += "\n\n"
			}
			text += fmt.Sprintf("<file name=%q>\n%s\n</file>", att.Name, content)
		}
	}

	if len(parts) == 0 {
		return ai.Message{Role: role, Content: text}
	}
	if text != "" {
		parts = append([]ai.ContentPart{ai.TextPart(text)}, parts...)
	}
	return ai.Message{Role: role, Content: parts}
}

// generateTitle asks the title model for a short thread title and falls back to
// the chat model, then to the first user message, when no title is produced.
func (a *App) generateTitle(client *ai.Client, cfg config.Config, fallbackModel, content string) string {
	if strings.TrimSpace(content) == "" {
		return defaultThreadTitle
	}
	model := cfg.TitleModel
	if model == "" {
		model = fallbackModel
	}
	if model == "" {
		model = cfg.ChatModel
	}
	if model == "" {
		return titleFromContent(content)
	}

	generated := a.streamTitle(client, model, content)
	if strings.TrimSpace(generated) == "" && fallbackModel != "" && fallbackModel != model {
		// Reasoning models can spend the whole budget on hidden reasoning and
		// return no content; retry with the chat model before giving up.
		generated = a.streamTitle(client, fallbackModel, content)
	}
	return normalizeTitle(generated, content)
}

func (a *App) streamTitle(client *ai.Client, model, content string) string {
	temperature := titleTemperature
	maxTokens := titleMaxTokens
	ctx, cancel := context.WithTimeout(a.ctx, titleTimeout)
	defer cancel()
	var generated strings.Builder
	err := client.StreamChat(ctx, ai.ChatOptions{
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
		log.Printf("app: generate title with %s: %v", model, err)
		return ""
	}
	return generated.String()
}

func normalizeTitle(raw, content string) string {
	title := strings.TrimSpace(raw)
	title = strings.TrimSpace(strings.TrimPrefix(title, "Title:"))
	title = strings.TrimSpace(strings.TrimPrefix(title, "title:"))
	title = strings.TrimSpace(strings.Trim(title, "\"'`*"))
	title = strings.Join(strings.Fields(title), " ")
	title = strings.TrimRight(title, ".!?,;:")
	lower := strings.ToLower(title)
	for _, prefix := range []string{"i'm sorry", "i am sorry", "i cannot", "i can't", "as an ai"} {
		if strings.HasPrefix(lower, prefix) {
			return titleFromContent(content)
		}
	}
	if title == "" {
		return titleFromContent(content)
	}
	return truncateRunes(title, 60)
}

func titleFromContent(content string) string {
	text := strings.TrimSpace(content)
	// Use the first non-empty line and drop common Markdown prefixes.
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			text = line
			break
		}
	}
	text = strings.TrimSpace(strings.TrimPrefix(text, "```"))
	text = strings.TrimLeft(text, "#>-*` ")
	text = strings.Trim(text, "\"'`*_")
	words := strings.Fields(text)
	if len(words) == 0 {
		return defaultThreadTitle
	}
	if len(words) > 8 {
		words = words[:8]
	}
	title := strings.TrimRight(strings.Join(words, " "), ".,!?;:")
	title = truncateRunes(title, 60)
	runes := []rune(title)
	if len(runes) == 0 {
		return defaultThreadTitle
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return s
}
