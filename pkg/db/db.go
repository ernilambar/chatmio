// Package db owns the SQLite chat store.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS threads (
	id                TEXT PRIMARY KEY,
	title             TEXT    NOT NULL DEFAULT 'New chat',
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

CREATE INDEX IF NOT EXISTS idx_messages_thread
	ON messages (thread_id, created_at);
`

// Thread is a persisted conversation.
type Thread struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
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
	ID        string `json:"id"`
	ThreadID  string `json:"threadId"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"createdAt"`
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

	return &Store{db: sqlDB}, nil
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
			id, title, system_prompt, temperature, top_p,
			max_tokens, frequency_penalty, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Title, t.SystemPrompt, t.Temperature, t.TopP,
		t.MaxTokens, t.FrequencyPenalty, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("db: create thread: %w", err)
	}
	return nil
}

// ListThreads returns threads ordered by most recently updated.
func (s *Store) ListThreads() ([]Thread, error) {
	rows, err := s.db.Query(`
		SELECT id, title, system_prompt, temperature, top_p,
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
			&t.ID, &t.Title, &t.SystemPrompt, &t.Temperature, &t.TopP,
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
		SELECT id, title, system_prompt, temperature, top_p,
		       max_tokens, frequency_penalty, created_at, updated_at
		FROM threads WHERE id = ?`, id).Scan(
		&t.ID, &t.Title, &t.SystemPrompt, &t.Temperature, &t.TopP,
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

// AddMessage inserts a message.
func (s *Store) AddMessage(m Message) error {
	_, err := s.db.Exec(
		`INSERT INTO messages (id, thread_id, role, content, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		m.ID, m.ThreadID, m.Role, m.Content, m.CreatedAt)
	if err != nil {
		return fmt.Errorf("db: add message: %w", err)
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
	defer rows.Close()

	messages := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: scan message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
