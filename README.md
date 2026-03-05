# flaky-servy

A tiny HTTP server that lists and serves YAML config files for a CLI.

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

## Errors

Errors return JSON:

```json
{
  "code": "invalid_name",
  "message": "config name must be a case-sensitive basename ending in .yaml or .yml"
}
```

Error codes:
- `invalid_name` (`400`)
- `not_found` (`404`)
- `internal_error` (`500`)

## Run

```bash
go run ./cmd/flaky-servy -addr :8080 -config-dir ./configs
```

## Quick Try

```bash
curl -s http://localhost:8080/configs | jq
curl -i http://localhost:8080/configs/go-example.yaml
```
