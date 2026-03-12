package config

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGetRejectsTraversalName(t *testing.T) {
	s := NewStore(t.TempDir())

	_, err := s.Get(context.Background(), "../evil.yaml")
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected ErrInvalidName for traversal, got %v", err)
	}
}

func TestGetAllowsCaseSensitiveYAMLExtensionsOnly(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	if _, err := s.Get(context.Background(), "demo.YAML"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected ErrInvalidName for uppercase extension, got %v", err)
	}

	path := filepath.Join(dir, "demo.yaml")
	content := []byte("name: demo\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	file, err := s.Get(context.Background(), "demo.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.Name != "demo.yaml" {
		t.Fatalf("expected demo.yaml, got %s", file.Name)
	}
}

func TestPutCreatesAndOverwritesConfig(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	created, err := s.Put(context.Background(), "demo.yaml", []byte("name: one\n"))
	if err != nil {
		t.Fatalf("unexpected error on create: %v", err)
	}
	if !created {
		t.Fatal("expected created=true on first write")
	}

	created, err = s.Put(context.Background(), "demo.yaml", []byte("name: two\n"))
	if err != nil {
		t.Fatalf("unexpected error on overwrite: %v", err)
	}
	if created {
		t.Fatal("expected created=false on overwrite")
	}

	content, err := os.ReadFile(filepath.Join(dir, "demo.yaml"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if !bytes.Equal(content, []byte("name: two\n")) {
		t.Fatalf("unexpected content: %q", string(content))
	}
}

func TestPutRejectsInvalidName(t *testing.T) {
	s := NewStore(t.TempDir())

	_, err := s.Put(context.Background(), "../evil.yaml", []byte("x: 1\n"))
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected ErrInvalidName for traversal, got %v", err)
	}

	_, err = s.Put(context.Background(), "evil.txt", []byte("x: 1\n"))
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected ErrInvalidName for unsupported extension, got %v", err)
	}
}
