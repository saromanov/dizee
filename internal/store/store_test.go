package store

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "nested", "store.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()
	original := []byte("first\n\x00\n\nlast\n")
	handle, err := s.Put(original)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if handle != Handle(original) {
		t.Errorf("Put handle = %q, want %q", handle, Handle(original))
	}
	if len(handle) != 64 {
		t.Errorf("handle length = %d, want 64", len(handle))
	}
	got, err := s.Get(handle)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("Get = %q, want exact original %q", got, original)
	}
}

func TestStorePutIsIdempotent(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()
	content := []byte("same content\n")
	first, err := s.Put(content)
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}
	second, err := s.Put(content)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if first != second {
		t.Errorf("handles differ: %q != %q", first, second)
	}
}

func TestStoreReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	content := []byte("persistent\n")
	handle, err := s.Put(content)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = s.Close() }()
	got, err := s.Get(handle)
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("Get = %q, want %q", got, content)
	}
}

func TestStoreNotFoundAndInvalidHandle(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Get("0000000000000000000000000000000000000000000000000000000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing handle error = %v, want ErrNotFound", err)
	}
	for _, handle := range []string{"", "xyz", "000000000000000000000000000000000000000000000000000000000000000G"} {
		if _, err := s.Get(handle); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%q) error = %v, want invalid handle", handle, err)
		}
	}
}

func TestStoreFullReturnsError(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.db.Exec(`CREATE TRIGGER reject_insert BEFORE INSERT ON blobs
		BEGIN SELECT RAISE(FAIL, 'store full'); END`); err != nil {
		t.Fatalf("create reject trigger: %v", err)
	}
	if _, err := s.Put([]byte("cannot save")); err == nil {
		t.Fatal("Put succeeded with a full store")
	}
}
