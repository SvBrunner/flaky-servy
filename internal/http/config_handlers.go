package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	configstore "github.com/SvBrunner/flaky-servy/internal/config"
)

type Handler struct {
	store *configstore.Store
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewHandler(store *configstore.Store) http.Handler {
	h := &Handler{store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /configs", h.listConfigs)
	mux.HandleFunc("GET /configs/{name}", h.downloadConfig)
	return mux
}

func (h *Handler) listConfigs(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.List(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to list configs")
		return
	}

	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) downloadConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	file, err := h.store.Get(r.Context(), name)
	if err != nil {
		switch {
		case errors.Is(err, configstore.ErrInvalidName):
			writeJSONError(w, http.StatusBadRequest, "invalid_name", "config name must be a case-sensitive basename ending in .yaml or .yml")
		case errors.Is(err, configstore.ErrNotFound):
			writeJSONError(w, http.StatusNotFound, "not_found", "config not found")
		default:
			writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to read config")
		}
		return
	}

	headETag := formatETagHeader(file.ETag)
	if ifNoneMatchMatches(r.Header.Get("If-None-Match"), headETag) {
		w.Header().Set("ETag", headETag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("ETag", headETag)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Content)
}

func ifNoneMatchMatches(ifNoneMatchHeader, etag string) bool {
	if ifNoneMatchHeader == "" {
		return false
	}
	if strings.TrimSpace(ifNoneMatchHeader) == "*" {
		return true
	}
	for part := range strings.SplitSeq(ifNoneMatchHeader, ",") {
		if strings.TrimSpace(part) == etag {
			return true
		}
	}
	return false
}

func formatETagHeader(etag string) string {
	return fmt.Sprintf("\"%s\"", etag)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{
		Code:    code,
		Message: message,
	})
}
