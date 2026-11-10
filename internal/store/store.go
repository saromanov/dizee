// Package store keeps original input bytes for recovery after lossy transforms.
package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("recovery handle not found")

type Store struct {
	db *sql.DB
}

func OpenDefault() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find home directory: %w", err)
	}
	return Open(filepath.Join(home, ".dizee", "store.db"))
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create store directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 1000`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure store: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS blobs (
		handle TEXT PRIMARY KEY,
		content BLOB NOT NULL
	) WITHOUT ROWID`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize store: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func Handle(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func (s *Store) Put(content []byte) (string, error) {
	handle := Handle(content)
	if _, err := s.db.Exec(`INSERT INTO blobs (handle, content) VALUES (?, ?)
		ON CONFLICT(handle) DO NOTHING`, handle, content); err != nil {
		return "", fmt.Errorf("save original: %w", err)
	}
	return handle, nil
}

func (s *Store) Get(handle string) ([]byte, error) {
	if !validHandle(handle) {
		return nil, fmt.Errorf("invalid recovery handle %q", handle)
	}
	var content []byte
	err := s.db.QueryRow(`SELECT content FROM blobs WHERE handle = ?`, handle).Scan(&content)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("retrieve original: %w", err)
	}
	return content, nil
}

func validHandle(handle string) bool {
	if len(handle) != sha256.Size*2 || strings.Trim(handle, "0123456789abcdef") != "" {
		return false
	}
	return true
}
