package config

import (
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
