// Package db owns the SQLite chat store.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS threads (
	id                TEXT PRIMARY KEY,
	title             TEXT    NOT NULL DEFAULT 'New chat',
	model             TEXT    NOT NULL DEFAULT '',
	system_prompt     TEXT    NOT NULL DEFAULT '',
	temperature       REAL    NOT NULL DEFAULT 0.7,
	top_p             REAL    NOT NULL DEFAULT 1.0,
	max_tokens        INTEGER NOT NULL DEFAULT 2048,
	frequency_penalty REAL    NOT NULL DEFAULT 0.0,
	created_at        INTEGER NOT NULL,
	updated_at        INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
	id         TEXT PRIMARY KEY,
	thread_id  TEXT    NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
	role       TEXT    NOT NULL CHECK (role IN ('system', 'user', 'assistant')),
	content    TEXT    NOT NULL,
	created_at INTEGER NOT NULL
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

// Thread is a persisted conversation.
type Thread struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	Model            string  `json:"model"`
	SystemPrompt     string  `json:"systemPrompt"`
	Temperature      float64 `json:"temperature"`
	TopP             float64 `json:"topP"`
	MaxTokens        int     `json:"maxTokens"`
	FrequencyPenalty float64 `json:"frequencyPenalty"`
	CreatedAt        int64   `json:"createdAt"`
	UpdatedAt        int64   `json:"updatedAt"`
}

// Message is a persisted chat message.
type Message struct {
	ID          string       `json:"id"`
	ThreadID    string       `json:"threadId"`
	Role        string       `json:"role"`
	Content     string       `json:"content"`
	Attachments []Attachment `json:"attachments,omitempty"`
	CreatedAt   int64        `json:"createdAt"`
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
		return nil, fmt.Errorf("db: migrate: %w", err)
	}
	if err := ensureColumn(sqlDB, "threads", "model", "TEXT NOT NULL DEFAULT ''"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("db: migrate threads.model: %w", err)
	}

	return &Store{db: sqlDB}, nil
}

// ensureColumn adds column when an existing table predates it.
func ensureColumn(sqlDB *sql.DB, table, column, definition string) error {
	rows, err := sqlDB.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var (
			cid        int
			name       string
			ctype      string
			notnull    int
			defaultV   any
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &defaultV, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = sqlDB.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
	return err
}

// Close releases the database connection.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return s.db.Close()
}

// CreateThread inserts a thread.
func (s *Store) CreateThread(t Thread) error {
	_, err := s.db.Exec(`
		INSERT INTO threads (
			id, title, model, system_prompt, temperature, top_p,
			max_tokens, frequency_penalty, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Title, t.Model, t.SystemPrompt, t.Temperature, t.TopP,
		t.MaxTokens, t.FrequencyPenalty, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("db: create thread: %w", err)
	}
	return nil
}

// ListThreads returns threads ordered by most recently updated.
func (s *Store) ListThreads() ([]Thread, error) {
	rows, err := s.db.Query(`
		SELECT id, title, model, system_prompt, temperature, top_p,
		       max_tokens, frequency_penalty, created_at, updated_at
		FROM threads
		ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("db: list threads: %w", err)
	}
	defer rows.Close()

	threads := []Thread{}
	for rows.Next() {
		var t Thread
		if err := rows.Scan(
			&t.ID, &t.Title, &t.Model, &t.SystemPrompt, &t.Temperature, &t.TopP,
			&t.MaxTokens, &t.FrequencyPenalty, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("db: scan thread: %w", err)
		}
		threads = append(threads, t)
	}
	return threads, rows.Err()
}

// GetThread returns a single thread.
func (s *Store) GetThread(id string) (Thread, error) {
	var t Thread
	err := s.db.QueryRow(`
		SELECT id, title, model, system_prompt, temperature, top_p,
		       max_tokens, frequency_penalty, created_at, updated_at
		FROM threads WHERE id = ?`, id).Scan(
		&t.ID, &t.Title, &t.Model, &t.SystemPrompt, &t.Temperature, &t.TopP,
		&t.MaxTokens, &t.FrequencyPenalty, &t.CreatedAt, &t.UpdatedAt)
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

// SetThreadModel updates the model used by a thread.
func (s *Store) SetThreadModel(id, model string) error {
	_, err := s.db.Exec(`UPDATE threads SET model = ? WHERE id = ?`, model, id)
	if err != nil {
		return fmt.Errorf("db: set thread model: %w", err)
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

	if _, err := tx.Exec(
		`INSERT INTO messages (id, thread_id, role, content, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		m.ID, m.ThreadID, m.Role, m.Content, m.CreatedAt); err != nil {
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

// ListMessages returns a thread's messages in chronological order.
func (s *Store) ListMessages(threadID string) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT id, thread_id, role, content, created_at
		FROM messages
		WHERE thread_id = ?
		ORDER BY created_at`, threadID)
	if err != nil {
		return nil, fmt.Errorf("db: list messages: %w", err)
	}

	messages := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("db: scan message: %w", err)
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
		ORDER BY created_at`, threadID)
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
