package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidName = errors.New("invalid config file name")
	ErrNotFound    = errors.New("config file not found")
)

type Item struct {
	Name         string `json:"name"`
	LastModified string `json:"lastModified"`
	ETag         string `json:"etag"`
}

type File struct {
	Name         string
	Path         string
	Content      []byte
	LastModified time.Time
	ETag         string
}

type Store struct {
	dir string
}

func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) List(ctx context.Context) ([]Item, error) {
	_ = ctx

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read config directory: %w", err)
	}

	items := make([]Item, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !isSupportedExtension(name) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("read file info for %q: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}

		content, err := os.ReadFile(filepath.Join(s.dir, name))
		if err != nil {
			return nil, fmt.Errorf("read config file %q: %w", name, err)
		}

		items = append(items, Item{
			Name:         name,
			LastModified: formatRFC3339UTCSeconds(info.ModTime()),
			ETag:         hashContent(content),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	return items, nil
}

func (s *Store) Get(ctx context.Context, name string) (*File, error) {
	_ = ctx

	if !isSafeName(name) {
		return nil, ErrInvalidName
	}
	if !isSupportedExtension(name) {
		return nil, ErrInvalidName
	}

	path := filepath.Join(s.dir, name)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("stat config file %q: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return nil, ErrNotFound
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("read config file %q: %w", name, err)
	}

	return &File{
		Name:         name,
		Path:         path,
		Content:      content,
		LastModified: info.ModTime().UTC().Truncate(time.Second),
		ETag:         hashContent(content),
	}, nil
}

func (s *Store) Put(ctx context.Context, name string, content []byte) (bool, error) {
	_ = ctx

	if !isSafeName(name) {
		return false, ErrInvalidName
	}
	if !isSupportedExtension(name) {
		return false, ErrInvalidName
	}

	path := filepath.Join(s.dir, name)
	created := true
	info, err := os.Stat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return false, ErrInvalidName
		}
		created = false
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("stat config file %q: %w", name, err)
	}

	if err := os.WriteFile(path, content, 0o644); err != nil {
		return false, fmt.Errorf("write config file %q: %w", name, err)
	}

	return created, nil
}

func isSafeName(name string) bool {
	if name == "" {
		return false
	}
	if name != filepath.Base(name) {
		return false
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	return true
}

func isSupportedExtension(name string) bool {
	ext := filepath.Ext(name)
	return ext == ".yaml" || ext == ".yml"
}

func formatRFC3339UTCSeconds(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

func hashContent(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}
