# flaky-servy

A tiny HTTP server that lists, serves, and uploads YAML config files for a CLI.

## Endpoints

### `GET /configs`
Returns available config files sorted by case-sensitive filename ascending.

Response body (`application/json`):

```json
[
  {
    "name": "go-example.yaml",
    "lastModified": "2026-03-05T12:34:56Z",
    "etag": "sha256:..."
  }
]
```

- `lastModified` is RFC3339 UTC with second precision.
- `etag` is a hash-based SHA-256 tag over file bytes.
- Only `.yaml` and `.yml` files are listed.

### `POST /configs`
Creates or overwrites one config file from JSON. Requires OIDC bearer auth.

Request body (`application/json`):

```json
{
  "name": "go-example.yaml",
  "content": "Name: Go dev environment\nLanguage: Go\n"
}
```

Rules:
- Name must be a basename (no path separators).
- Extension must be exactly `.yaml` or `.yml` (case-sensitive).

Responses:
- `201 Created` when file is created.
- `200 OK` when an existing file is overwritten.

Response body (`application/json`):

```json
{
  "name": "go-example.yaml",
  "created": true,
  "lastModified": "2026-03-05T12:34:56Z",
  "etag": "sha256:..."
}
```

Response headers:
- `ETag: "sha256:..."`

Auth:
- Header: `Authorization: Bearer <token>`
- Token must validate against configured issuer and include the configured audience.

### `GET /configs/{name}`
Downloads one config file by case-sensitive name.

Rules:
- Name must be a basename (no path separators).
- Extension must be exactly `.yaml` or `.yml` (case-sensitive).
- Files that differ only by case are valid and treated as distinct names.

Response headers:
- `Content-Type: application/yaml; charset=utf-8`
- `ETag: "sha256:..."`

Conditional requests:
- Send `If-None-Match` with the ETag value.
- Server returns `304 Not Modified` when it matches.

### `GET /upload`
Serves a simple upload UI:
- Login with OIDC.
- List existing configs.
- Upload by filename + YAML free text.

### `GET /oidc/login`
Starts OIDC Authorization Code with PKCE flow for the upload UI.

### `GET /oidc/callback`
OIDC callback for the upload UI login flow.

## Errors

Errors return JSON:

```json
{
  "code": "invalid_name",
  "message": "config name must be a case-sensitive basename ending in .yaml or .yml"
}
```

Error codes:
- `invalid_body` (`400`)
- `invalid_name` (`400`)
- `unauthorized` (`401`)
- `not_found` (`404`)
- `internal_error` (`500`)

## Configuration

Required environment variables:
- `OIDC_ISSUER`: OIDC issuer URL.
- `OIDC_AUDIENCE`: required token audience for upload API.
- `OIDC_CLIENT_ID`: OIDC client id used for UI login flow.
- `OIDC_CLIENT_SECRET`: OIDC client secret used for authorization code exchange and token introspection.

Optional environment variables:
- `OIDC_REDIRECT_URI`: callback URL. Default: `http://localhost:8080/oidc/callback`

## Run

```bash
export OIDC_ISSUER="https://issuer.example.com/"
export OIDC_AUDIENCE="flaky-servy-api"
export OIDC_CLIENT_ID="flaky-servy-web"
export OIDC_CLIENT_SECRET="replace-me"
# optional
export OIDC_REDIRECT_URI="http://localhost:8080/oidc/callback"

go run ./cmd/flaky-servy -addr :8080 -config-dir ./configs
```

## Quick Try

```bash
curl -s http://localhost:8080/configs | jq
curl -i http://localhost:8080/configs/go-example.yaml

# with a valid bearer token
curl -i -X POST http://localhost:8080/configs \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"name":"demo.yaml","content":"name: demo\n"}'
```
