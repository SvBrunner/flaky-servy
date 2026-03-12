package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	configstore "github.com/SvBrunner/flaky-servy/internal/config"
)

type Handler struct {
	store *configstore.Store
	auth  *OIDCAuth
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type uploadRequest struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type uploadResponse struct {
	Name         string `json:"name"`
	Created      bool   `json:"created"`
	LastModified string `json:"lastModified"`
	ETag         string `json:"etag"`
}

const accessTokenStorageKey = "flaky_servy_access_token"

func NewHandler(store *configstore.Store) http.Handler {
	return newHandler(store, nil)
}

func NewHandlerWithOIDC(store *configstore.Store, auth *OIDCAuth) http.Handler {
	return newHandler(store, auth)
}

func newHandler(store *configstore.Store, auth *OIDCAuth) http.Handler {
	h := &Handler{store: store, auth: auth}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /configs", h.listConfigs)
	mux.HandleFunc("POST /configs", h.uploadConfig)
	mux.HandleFunc("GET /configs/{name}", h.downloadConfig)
	mux.HandleFunc("GET /upload", h.uploadPage)
	mux.HandleFunc("GET /oidc/login", h.oidcLogin)
	mux.HandleFunc("GET /oidc/callback", h.oidcCallback)
	return mux
}

func (h *Handler) listConfigs(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.List(r.Context())
	if err != nil {
		logRequestError(r, "failed to list configs", err)
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to list configs")
		return
	}

	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) uploadConfig(w http.ResponseWriter, r *http.Request) {
	if !h.requireBearer(w, r) {
		return
	}

	defer r.Body.Close()
	var req uploadRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		logRequestError(r, "invalid upload json body", err)
		writeJSONError(w, http.StatusBadRequest, "invalid_body", "request body must be valid JSON")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		logRequestError(r, "upload body contains extra json values", err)
		writeJSONError(w, http.StatusBadRequest, "invalid_body", "request body must contain one JSON object")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		logRequestError(r, "missing config name", nil)
		writeJSONError(w, http.StatusBadRequest, "invalid_name", "config name must be a case-sensitive basename ending in .yaml or .yml")
		return
	}

	created, err := h.store.Put(r.Context(), req.Name, []byte(req.Content))
	if err != nil {
		switch {
		case errors.Is(err, configstore.ErrInvalidName):
			logRequestError(r, "invalid config name", err)
			writeJSONError(w, http.StatusBadRequest, "invalid_name", "config name must be a case-sensitive basename ending in .yaml or .yml")
		default:
			logRequestError(r, "failed to write config", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to write config")
		}
		return
	}

	file, err := h.store.Get(r.Context(), req.Name)
	if err != nil {
		logRequestError(r, "failed to read written config", err)
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to read written config")
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	w.Header().Set("ETag", formatETagHeader(file.ETag))
	writeJSON(w, status, uploadResponse{
		Name:         file.Name,
		Created:      created,
		LastModified: file.LastModified.UTC().Truncate(time.Second).Format(time.RFC3339),
		ETag:         file.ETag,
	})
}

func (h *Handler) downloadConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	file, err := h.store.Get(r.Context(), name)
	if err != nil {
		switch {
		case errors.Is(err, configstore.ErrInvalidName):
			logRequestError(r, "invalid config name", err)
			writeJSONError(w, http.StatusBadRequest, "invalid_name", "config name must be a case-sensitive basename ending in .yaml or .yml")
		case errors.Is(err, configstore.ErrNotFound):
			logRequestError(r, "config not found", err)
			writeJSONError(w, http.StatusNotFound, "not_found", "config not found")
		default:
			logRequestError(r, "failed to read config", err)
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

func (h *Handler) uploadPage(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		logRequestError(r, "oidc auth is not configured", nil)
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "oidc auth is not configured")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(uploadPageHTML))
}

func (h *Handler) oidcLogin(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		logRequestError(r, "oidc auth is not configured", nil)
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "oidc auth is not configured")
		return
	}

	state, err := randomBase64URL(24)
	if err != nil {
		logRequestError(r, "failed to initialize oidc login state", err)
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to initialize oidc login")
		return
	}
	verifier, err := randomBase64URL(32)
	if err != nil {
		logRequestError(r, "failed to initialize oidc login verifier", err)
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to initialize oidc login")
		return
	}

	http.SetCookie(w, &http.Cookie{Name: "flaky_servy_oidc_state", Value: state, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 300})
	http.SetCookie(w, &http.Cookie{Name: "flaky_servy_oidc_verifier", Value: verifier, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 300})

	http.Redirect(w, r, h.auth.AuthCodeURL(state, codeChallengeS256(verifier)), http.StatusFound)
}

func (h *Handler) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		logRequestError(r, "oidc auth is not configured", nil)
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "oidc auth is not configured")
		return
	}

	if oidcErr := r.URL.Query().Get("error"); oidcErr != "" {
		logRequestError(r, "oidc provider returned error", fmt.Errorf("%s", oidcErr))
		h.redirectUploadError(w, r, "oidc login failed")
		return
	}

	stateCookie, err := r.Cookie("flaky_servy_oidc_state")
	if err != nil || r.URL.Query().Get("state") == "" || stateCookie.Value != r.URL.Query().Get("state") {
		if err != nil {
			logRequestError(r, "missing oidc state cookie", err)
		} else {
			logRequestError(r, "invalid oidc callback state", nil)
		}
		h.redirectUploadError(w, r, "invalid oidc callback state")
		return
	}
	verifierCookie, err := r.Cookie("flaky_servy_oidc_verifier")
	if err != nil {
		logRequestError(r, "missing oidc verifier cookie", err)
		h.redirectUploadError(w, r, "missing oidc code verifier")
		return
	}

	token, err := h.auth.Exchange(r.Context(), r.URL.Query().Get("code"), verifierCookie.Value)
	if err != nil || token.AccessToken == "" {
		logRequestError(r, "failed to exchange authorization code", err)
		h.redirectUploadError(w, r, "failed to exchange authorization code")
		return
	}
	if err := h.auth.VerifyAccessToken(r.Context(), token.AccessToken); err != nil {
		logRequestError(r, "token validation failed", err)
		h.redirectUploadError(w, r, "token validation failed")
		return
	}

	http.SetCookie(w, &http.Cookie{Name: "flaky_servy_oidc_state", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "flaky_servy_oidc_verifier", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})

	tokenJSON, _ := json.Marshal(token.AccessToken)
	tmpl := template.Must(template.New("callback").Parse(oidcCallbackHTML))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.Execute(w, struct {
		Token template.JS
	}{Token: template.JS(tokenJSON)})
}

func (h *Handler) redirectUploadError(w http.ResponseWriter, r *http.Request, message string) {
	http.Redirect(w, r, "/upload?error="+url.QueryEscape(message), http.StatusFound)
}

func (h *Handler) requireBearer(w http.ResponseWriter, r *http.Request) bool {
	if h.auth == nil {
		return true
	}

	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		logRequestError(r, "missing bearer token", nil)
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if token == "" {
		logRequestError(r, "empty bearer token", nil)
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
		return false
	}
	if err := h.auth.VerifyAccessToken(r.Context(), token); err != nil {
		logRequestError(r, "invalid bearer token", err)
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
		return false
	}
	return true
}

func logRequestError(r *http.Request, message string, err error) {
	if err != nil {
		log.Printf("http error method=%s path=%s remote=%s message=%s err=%v", r.Method, r.URL.Path, r.RemoteAddr, message, err)
		return
	}
	log.Printf("http error method=%s path=%s remote=%s message=%s", r.Method, r.URL.Path, r.RemoteAddr, message)
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
	writeJSON(w, status, errorResponse{Code: code, Message: message})
}

const uploadPageHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>flaky-servy upload</title>
  <style>
    body { font-family: sans-serif; max-width: 900px; margin: 2rem auto; padding: 0 1rem; }
    textarea { width: 100%; min-height: 280px; font-family: monospace; }
    input[type="text"] { width: 100%; }
    .row { margin: 0.75rem 0; }
    .hidden { display: none; }
    pre { background: #f4f4f4; padding: 0.75rem; overflow: auto; }
  </style>
</head>
<body>
  <h1>Config Upload</h1>
  <div id="auth-box" class="row">
    <a href="/oidc/login">Login with OIDC</a>
  </div>

  <div id="app" class="hidden">
    <div class="row">
      <button id="logout" type="button">Logout</button>
    </div>
    <div class="row">
      <h2>Existing configs</h2>
      <ul id="configs"></ul>
    </div>

    <div class="row">
      <label for="name">Filename (.yaml/.yml)</label>
      <input id="name" type="text" placeholder="example.yaml" />
    </div>
    <div class="row">
      <label for="content">YAML</label>
      <textarea id="content" placeholder="name: value"></textarea>
    </div>
    <div class="row">
      <button id="submit" type="button">Upload</button>
    </div>

    <div class="row">
      <strong>Result</strong>
      <pre id="result"></pre>
    </div>
  </div>

<script>
(() => {
  const key = "` + accessTokenStorageKey + `";
  const token = sessionStorage.getItem(key);
  const authBox = document.getElementById("auth-box");
  const app = document.getElementById("app");
  const result = document.getElementById("result");

  function setResult(text) {
    result.textContent = text;
  }

  function showLoggedOut() {
    authBox.classList.remove("hidden");
    app.classList.add("hidden");
  }

  async function loadConfigs() {
    const res = await fetch("/configs");
    if (!res.ok) {
      setResult("Failed to fetch configs: " + res.status);
      return;
    }
    const data = await res.json();
    const ul = document.getElementById("configs");
    ul.innerHTML = "";
    for (const item of data) {
      const li = document.createElement("li");
      li.textContent = item.name + " (" + item.lastModified + ")";
      ul.appendChild(li);
    }
  }

  async function upload() {
    const name = document.getElementById("name").value;
    const content = document.getElementById("content").value;

    const res = await fetch("/configs", {
      method: "POST",
      headers: {
        "Authorization": "Bearer " + sessionStorage.getItem(key),
        "Content-Type": "application/json"
      },
      body: JSON.stringify({ name, content })
    });

    const text = await res.text();
    if (res.status === 401) {
      sessionStorage.removeItem(key);
      showLoggedOut();
    }
    setResult("HTTP " + res.status + "\n" + text);
    if (res.ok) {
      await loadConfigs();
    }
  }

  const params = new URLSearchParams(window.location.search);
  const callbackError = params.get("error");
  if (callbackError) {
    setResult(callbackError);
    sessionStorage.removeItem(key);
  }

  if (!token || callbackError) {
    showLoggedOut();
    return;
  }

  authBox.classList.add("hidden");
  app.classList.remove("hidden");
  document.getElementById("submit").addEventListener("click", upload);
  document.getElementById("logout").addEventListener("click", () => {
    sessionStorage.removeItem(key);
    showLoggedOut();
  });
  loadConfigs();
})();
</script>
</body>
</html>`

const oidcCallbackHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>OIDC login</title></head>
<body>
<script>
  sessionStorage.setItem("` + accessTokenStorageKey + `", {{ .Token }});
  window.location.replace("/upload");
</script>
</body>
</html>`
