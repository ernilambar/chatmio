// Package db owns the SQLite chat store.
package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Nullable thread generation settings mean the provider default; they are left
// out of the request.
const schema = `
CREATE TABLE IF NOT EXISTS threads (
	id                TEXT PRIMARY KEY,
	title             TEXT    NOT NULL DEFAULT 'New chat',
	model             TEXT    NOT NULL DEFAULT '',
	system_prompt     TEXT    NOT NULL DEFAULT '',
	temperature       REAL,
	top_p             REAL,
	max_tokens        INTEGER,
	frequency_penalty REAL,
	reasoning_effort  TEXT    NOT NULL DEFAULT '',
	context_window    INTEGER,
	web_tools         INTEGER NOT NULL DEFAULT 0,
	created_at        INTEGER NOT NULL,
	updated_at        INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
	id                TEXT PRIMARY KEY,
	thread_id         TEXT    NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
	role              TEXT    NOT NULL CHECK (role IN ('system', 'user', 'assistant')),
	content           TEXT    NOT NULL,
	reasoning         TEXT    NOT NULL DEFAULT '',
	prompt_tokens     INTEGER NOT NULL DEFAULT 0,
	completion_tokens INTEGER NOT NULL DEFAULT 0,
	reasoning_tokens  INTEGER NOT NULL DEFAULT 0,
	tools             TEXT    NOT NULL DEFAULT '',
	created_at        INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS attachments (
	id         TEXT PRIMARY KEY,
	message_id TEXT    NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
	name       TEXT    NOT NULL,
	mime_type  TEXT    NOT NULL,
	size       INTEGER NOT NULL,
	data       BLOB    NOT NULL,
	created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_messages_thread
	ON messages (thread_id, created_at);

CREATE INDEX IF NOT EXISTS idx_attachments_message
	ON attachments (message_id, created_at);
`

// Thread is a persisted conversation. Nil generation settings use the
// provider default.
type Thread struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Model            string   `json:"model"`
	SystemPrompt     string   `json:"systemPrompt"`
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"topP,omitempty"`
	MaxTokens        *int     `json:"maxTokens,omitempty"`
	FrequencyPenalty *float64 `json:"frequencyPenalty,omitempty"`
	ReasoningEffort  string   `json:"reasoningEffort"`
	ContextWindow    *int     `json:"contextWindow,omitempty"`
	WebTools         bool     `json:"webTools"`
	CreatedAt        int64    `json:"createdAt"`
	UpdatedAt        int64    `json:"updatedAt"`
}

// ThreadSettings are the per-chat generation settings the user can edit.
type ThreadSettings struct {
	SystemPrompt     string   `json:"systemPrompt"`
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"topP,omitempty"`
	MaxTokens        *int     `json:"maxTokens,omitempty"`
	FrequencyPenalty *float64 `json:"frequencyPenalty,omitempty"`
	ReasoningEffort  string   `json:"reasoningEffort"`
	ContextWindow    *int     `json:"contextWindow,omitempty"`
}

// Message is a persisted chat message. Token counts are zero when the
// provider did not report usage.
type Message struct {
	ID               string       `json:"id"`
	ThreadID         string       `json:"threadId"`
	Role             string       `json:"role"`
	Content          string       `json:"content"`
	Reasoning        string       `json:"reasoning"`
	PromptTokens     int          `json:"promptTokens"`
	CompletionTokens int          `json:"completionTokens"`
	ReasoningTokens  int          `json:"reasoningTokens"`
	Attachments      []Attachment `json:"attachments,omitempty"`
	Tools            []ToolStep   `json:"tools,omitempty"`
	CreatedAt        int64        `json:"createdAt"`
}

// ToolStep records one tool call made while producing an assistant reply. It
// is kept for display; tool results are not replayed to the model.
type ToolStep struct {
	Name    string `json:"name"`
	Input   string `json:"input"`
	Summary string `json:"summary,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Attachment is a file stored alongside a user message.
type Attachment struct {
	ID        string `json:"id"`
	MessageID string `json:"messageId"`
	Name      string `json:"name"`
	MimeType  string `json:"mimeType"`
	Size      int64  `json:"size"`
	Data      []byte `json:"-"`
	DataURL   string `json:"dataUrl,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

// AttachmentInput is a file uploaded from the frontend.
type AttachmentInput struct {
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

// Store wraps the SQLite connection.
type Store struct {
	db *sql.DB
}

// Open opens (creating if necessary) the database at path and applies the schema.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("db: create dir: %w", err)
	}

	dsn := "file:" + path +
		"?_pragma=foreign_keys(1)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	// modernc/sqlite serialises writers best through a single connection.
	sqlDB.SetMaxOpenConns(1)

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	if _, err := sqlDB.Exec(schema); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("db: apply schema: %w", err)
	}
	return &Store{db: sqlDB}, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return s.db.Close()
}

const threadColumns = `id, title, model, system_prompt, temperature, top_p,
	max_tokens, frequency_penalty, reasoning_effort, context_window,
	web_tools, created_at, updated_at`

type scanner interface {
	Scan(dest ...any) error
}

func scanThread(row scanner) (Thread, error) {
	var t Thread
	err := row.Scan(
		&t.ID, &t.Title, &t.Model, &t.SystemPrompt, &t.Temperature, &t.TopP,
		&t.MaxTokens, &t.FrequencyPenalty, &t.ReasoningEffort, &t.ContextWindow,
		&t.WebTools, &t.CreatedAt, &t.UpdatedAt,
	)
	return t, err
}

// CreateThread inserts a thread.
func (s *Store) CreateThread(t Thread) error {
	_, err := s.db.Exec(`
		INSERT INTO threads (`+threadColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Title, t.Model, t.SystemPrompt, t.Temperature, t.TopP,
		t.MaxTokens, t.FrequencyPenalty, t.ReasoningEffort, t.ContextWindow,
		t.WebTools, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("db: create thread: %w", err)
	}
	return nil
}

// ListThreads returns threads ordered by most recently updated.
func (s *Store) ListThreads() ([]Thread, error) {
	rows, err := s.db.Query(`SELECT ` + threadColumns + ` FROM threads ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("db: list threads: %w", err)
	}
	defer rows.Close()

	threads := []Thread{}
	for rows.Next() {
		t, err := scanThread(rows)
		if err != nil {
			return nil, fmt.Errorf("db: scan thread: %w", err)
		}
		threads = append(threads, t)
	}
	return threads, rows.Err()
}

// SearchResult is a thread matching a search query with an excerpt from the
// most recent matching message.
type SearchResult struct {
	ThreadID string `json:"threadId"`
	Title    string `json:"title"`
	Snippet  string `json:"snippet"`
}

const (
	searchSnippetRunes = 160
	searchSnippetLead  = 40
)

// SearchThreads finds threads by title or message content, most recently
// updated first. Empty queries return no results.
func (s *Store) SearchThreads(query string, limit int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []SearchResult{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	pattern := "%" + escapeLike(query) + "%"

	rows, err := s.db.Query(`
		SELECT t.id, t.title
		FROM threads t
		WHERE t.title LIKE ? ESCAPE '\'
		   OR EXISTS (
			SELECT 1 FROM messages m
			WHERE m.thread_id = t.id AND m.content LIKE ? ESCAPE '\'
		   )
		ORDER BY t.updated_at DESC
		LIMIT ?`, pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("db: search threads: %w", err)
	}

	results := []SearchResult{}
	for rows.Next() {
		var result SearchResult
		if err := rows.Scan(&result.ThreadID, &result.Title); err != nil {
			rows.Close()
			return nil, fmt.Errorf("db: scan search result: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("db: close search rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db: search threads: %w", err)
	}

	for i := range results {
		snippet, err := s.searchSnippet(results[i].ThreadID, pattern, query)
		if err != nil {
			return nil, err
		}
		results[i].Snippet = snippet
	}
	return results, nil
}

func (s *Store) searchSnippet(threadID, pattern, query string) (string, error) {
	var content string
	err := s.db.QueryRow(`
		SELECT content FROM messages
		WHERE thread_id = ? AND content LIKE ? ESCAPE '\'
		ORDER BY created_at DESC, rowid DESC
		LIMIT 1`, threadID, pattern).Scan(&content)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("db: search snippet: %w", err)
	}
	return excerpt(content, query), nil
}

// excerpt collapses whitespace and centers a short window around the match.
func excerpt(content, query string) string {
	content = strings.Join(strings.Fields(content), " ")
	runes := []rune(content)
	if len(runes) <= searchSnippetRunes {
		return content
	}

	start := 0
	index := strings.Index(strings.ToLower(content), strings.ToLower(query))
	if index >= 0 {
		matchRune := len([]rune(content[:index]))
		start = matchRune - searchSnippetLead
		if start < 0 {
			start = 0
		}
	}
	end := start + searchSnippetRunes
	if end > len(runes) {
		end = len(runes)
		start = end - searchSnippetRunes
		if start < 0 {
			start = 0
		}
	}

	snippet := string(runes[start:end])
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(runes) {
		snippet += "…"
	}
	return snippet
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

// GetThread returns a single thread.
func (s *Store) GetThread(id string) (Thread, error) {
	t, err := scanThread(s.db.QueryRow(`SELECT `+threadColumns+` FROM threads WHERE id = ?`, id))
	if err != nil {
		return Thread{}, fmt.Errorf("db: get thread: %w", err)
	}
	return t, nil
}

// UpdateThreadMeta updates the title and updated timestamp.
func (s *Store) UpdateThreadMeta(id, title string, updatedAt int64) error {
	_, err := s.db.Exec(
		`UPDATE threads SET title = ?, updated_at = ? WHERE id = ?`,
		title, updatedAt, id)
	if err != nil {
		return fmt.Errorf("db: update thread: %w", err)
	}
	return nil
}

// TouchThread advances a thread's updated timestamp without changing its title.
func (s *Store) TouchThread(id string, updatedAt int64) error {
	_, err := s.db.Exec(`UPDATE threads SET updated_at = ? WHERE id = ?`, updatedAt, id)
	if err != nil {
		return fmt.Errorf("db: touch thread: %w", err)
	}
	return nil
}

// SetThreadTitleIfUnchanged replaces a title only while it still equals
// expected. It reports whether a row changed, so a user rename that lands while
// a generated title is in flight is never overwritten.
func (s *Store) SetThreadTitleIfUnchanged(id, expected, title string) (bool, error) {
	result, err := s.db.Exec(
		`UPDATE threads SET title = ? WHERE id = ? AND title = ?`,
		title, id, expected)
	if err != nil {
		return false, fmt.Errorf("db: set thread title: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("db: set thread title: %w", err)
	}
	return updated > 0, nil
}

// SetThreadModel updates the model used by a thread.
func (s *Store) SetThreadModel(id, model string) error {
	_, err := s.db.Exec(`UPDATE threads SET model = ? WHERE id = ?`, model, id)
	if err != nil {
		return fmt.Errorf("db: set thread model: %w", err)
	}
	return nil
}

// SetThreadWebTools turns web tools on or off for a thread.
func (s *Store) SetThreadWebTools(id string, enabled bool) error {
	_, err := s.db.Exec(`UPDATE threads SET web_tools = ? WHERE id = ?`, enabled, id)
	if err != nil {
		return fmt.Errorf("db: set thread web tools: %w", err)
	}
	return nil
}

// SetThreadSettings replaces a thread's system prompt and generation settings.
func (s *Store) SetThreadSettings(id string, settings ThreadSettings) error {
	_, err := s.db.Exec(`
		UPDATE threads SET
			system_prompt = ?, temperature = ?, top_p = ?, max_tokens = ?,
			frequency_penalty = ?, reasoning_effort = ?, context_window = ?
		WHERE id = ?`,
		settings.SystemPrompt, settings.Temperature, settings.TopP, settings.MaxTokens,
		settings.FrequencyPenalty, settings.ReasoningEffort, settings.ContextWindow, id)
	if err != nil {
		return fmt.Errorf("db: set thread settings: %w", err)
	}
	return nil
}

// DeleteThread removes a thread and its messages and attachments.
func (s *Store) DeleteThread(id string) error {
	_, err := s.db.Exec(`DELETE FROM threads WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("db: delete thread: %w", err)
	}
	return nil
}

// AddMessage inserts a message and its attachments.
func (s *Store) AddMessage(m Message) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("db: begin add message: %w", err)
	}
	defer tx.Rollback()

	tools := ""
	if len(m.Tools) > 0 {
		data, err := json.Marshal(m.Tools)
		if err != nil {
			return fmt.Errorf("db: encode tools: %w", err)
		}
		tools = string(data)
	}
	if _, err := tx.Exec(
		`INSERT INTO messages (
			id, thread_id, role, content, reasoning,
			prompt_tokens, completion_tokens, reasoning_tokens, tools, created_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ThreadID, m.Role, m.Content, m.Reasoning,
		m.PromptTokens, m.CompletionTokens, m.ReasoningTokens, tools, m.CreatedAt); err != nil {
		return fmt.Errorf("db: add message: %w", err)
	}

	for _, att := range m.Attachments {
		id := att.ID
		if id == "" {
			id = uuid.NewString()
		}
		messageID := att.MessageID
		if messageID == "" {
			messageID = m.ID
		}
		size := att.Size
		if size == 0 {
			size = int64(len(att.Data))
		}
		if _, err := tx.Exec(
			`INSERT INTO attachments (
				id, message_id, name, mime_type, size, data, created_at
			 ) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, messageID, att.Name, att.MimeType, size, att.Data, att.CreatedAt); err != nil {
			return fmt.Errorf("db: add attachment: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: commit message: %w", err)
	}
	return nil
}

// UpdateMessageContent replaces the text of one message.
func (s *Store) UpdateMessageContent(id, content string) error {
	_, err := s.db.Exec(`UPDATE messages SET content = ? WHERE id = ?`, content, id)
	if err != nil {
		return fmt.Errorf("db: update message: %w", err)
	}
	return nil
}

// DeleteMessagesAfter removes every message that follows messageID in the same
// thread. Attachments are removed by the foreign-key cascade.
func (s *Store) DeleteMessagesAfter(threadID, messageID string) error {
	_, err := s.db.Exec(`
		DELETE FROM messages
		WHERE thread_id = ?
		  AND rowid > (
			SELECT rowid FROM messages WHERE id = ? AND thread_id = ?
		  )`,
		threadID, messageID, threadID)
	if err != nil {
		return fmt.Errorf("db: delete following messages: %w", err)
	}
	return nil
}

// ListMessages returns a thread's messages in chronological order.
func (s *Store) ListMessages(threadID string) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT id, thread_id, role, content, reasoning,
		       prompt_tokens, completion_tokens, reasoning_tokens, tools, created_at
		FROM messages
		WHERE thread_id = ?
		ORDER BY created_at, rowid`, threadID)
	if err != nil {
		return nil, fmt.Errorf("db: list messages: %w", err)
	}

	messages := []Message{}
	for rows.Next() {
		var m Message
		var tools string
		if err := rows.Scan(
			&m.ID, &m.ThreadID, &m.Role, &m.Content, &m.Reasoning,
			&m.PromptTokens, &m.CompletionTokens, &m.ReasoningTokens, &tools, &m.CreatedAt,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("db: scan message: %w", err)
		}
		if tools != "" {
			if err := json.Unmarshal([]byte(tools), &m.Tools); err != nil {
				rows.Close()
				return nil, fmt.Errorf("db: decode tools: %w", err)
			}
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("db: list messages: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("db: close messages: %w", err)
	}
	if len(messages) == 0 {
		return messages, nil
	}

	attachmentRows, err := s.db.Query(`
		SELECT id, message_id, name, mime_type, size, data, created_at
		FROM attachments
		WHERE message_id IN (SELECT id FROM messages WHERE thread_id = ?)
		ORDER BY created_at, rowid`, threadID)
	if err != nil {
		return nil, fmt.Errorf("db: list attachments: %w", err)
	}
	defer attachmentRows.Close()

	byMessage := make(map[string][]Attachment, len(messages))
	for attachmentRows.Next() {
		var att Attachment
		if err := attachmentRows.Scan(
			&att.ID, &att.MessageID, &att.Name, &att.MimeType,
			&att.Size, &att.Data, &att.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("db: scan attachment: %w", err)
		}
		byMessage[att.MessageID] = append(byMessage[att.MessageID], att)
	}
	if err := attachmentRows.Err(); err != nil {
		return nil, fmt.Errorf("db: list attachments: %w", err)
	}

	for i := range messages {
		messages[i].Attachments = byMessage[messages[i].ID]
	}
	return messages, nil
}
