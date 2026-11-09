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
