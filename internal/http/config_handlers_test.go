package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	configstore "github.com/SvBrunner/flaky-servy/internal/config"
)

func TestListConfigsReturnsSortedItemsWithMetadata(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "b.yaml"), []byte("name: b\n"))
	mustWriteFile(t, filepath.Join(dir, "a.yml"), []byte("name: a\n"))
	mustWriteFile(t, filepath.Join(dir, "ignore.txt"), []byte("ignore\n"))

	fixedTime := time.Date(2026, 3, 5, 12, 34, 56, 123_000_000, time.UTC)
	for _, file := range []string{"b.yaml", "a.yml"} {
		if err := os.Chtimes(filepath.Join(dir, file), fixedTime, fixedTime); err != nil {
			t.Fatalf("chtimes %s: %v", file, err)
		}
	}

	h := NewHandler(configstore.NewStore(dir))
	req := httptest.NewRequest(http.MethodGet, "/configs", nil)
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}
	if got := res.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content-type: %q", got)
	}

	var items []configstore.Item
	if err := json.Unmarshal(res.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	names := []string{items[0].Name, items[1].Name}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("expected sorted names, got %v", names)
	}

	for _, item := range items {
		if item.LastModified != "2026-03-05T12:34:56Z" {
			t.Fatalf("expected seconds precision RFC3339 UTC timestamp, got %q", item.LastModified)
		}
		if item.ETag == "" {
			t.Fatalf("expected etag for %s", item.Name)
		}
	}
}

func TestDownloadConfigSupportsETagAnd304(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "demo.yaml"), []byte("name: demo\n"))

	h := NewHandler(configstore.NewStore(dir))
	firstReq := httptest.NewRequest(http.MethodGet, "/configs/demo.yaml", nil)
	firstRes := httptest.NewRecorder()
	h.ServeHTTP(firstRes, firstReq)

	if firstRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", firstRes.Code)
	}
	if got := firstRes.Header().Get("Content-Type"); got != "application/yaml; charset=utf-8" {
		t.Fatalf("unexpected content-type: %q", got)
	}
	etag := firstRes.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag header")
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/configs/demo.yaml", nil)
	secondReq.Header.Set("If-None-Match", etag)
	secondRes := httptest.NewRecorder()
	h.ServeHTTP(secondRes, secondReq)

	if secondRes.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", secondRes.Code)
	}
	if secondRes.Body.Len() != 0 {
		t.Fatalf("expected empty body for 304, got %q", secondRes.Body.String())
	}
}

func TestDownloadConfigRejectsInvalidNameAndMissingFile(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler(configstore.NewStore(dir))

	invalidReq := httptest.NewRequest(http.MethodGet, "/configs/evil.txt", nil)
	invalidRes := httptest.NewRecorder()
	h.ServeHTTP(invalidRes, invalidReq)
	assertErrorResponse(t, invalidRes, http.StatusBadRequest, "invalid_name")

	notFoundReq := httptest.NewRequest(http.MethodGet, "/configs/missing.yml", nil)
	notFoundRes := httptest.NewRecorder()
	h.ServeHTTP(notFoundRes, notFoundReq)
	assertErrorResponse(t, notFoundRes, http.StatusNotFound, "not_found")
}

func assertErrorResponse(t *testing.T, res *httptest.ResponseRecorder, expectedStatus int, expectedCode string) {
	t.Helper()

	if res.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d", expectedStatus, res.Code)
	}
	if got := res.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content-type: %q", got)
	}

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to parse error payload: %v", err)
	}
	if payload.Code != expectedCode {
		t.Fatalf("expected code %q, got %q", expectedCode, payload.Code)
	}
}

func mustWriteFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
