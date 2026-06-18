# PPDM API Simulator

A lightweight REST API simulator for **Dell PowerProtect Data Manager (PPDM)**. It loads the OpenAPI specifications from `openapi-json/` and returns schema-based mock responses for every documented endpoint (v1 license API, v2, and v3).

Written in **pure Go** (stdlib only).

## Quick start

```bash
go run ./cmd/ppdm-simulator
```

Or build with Make:

```bash
make build
./dist/ppdm-simulator
```

Binaries are written to `dist/` only — not the project root.

## Building

Requires [Go 1.22+](https://go.dev/dl/).

### Make targets

```bash
make build          # current OS/arch -> dist/ppdm-simulator
make release        # all platforms -> dist/
make linux-amd64    # dist/ppdm-simulator-linux-amd64
make linux-arm64    # dist/ppdm-simulator-linux-arm64
make darwin-amd64   # dist/ppdm-simulator-darwin-amd64
make darwin-arm64   # dist/ppdm-simulator-darwin-arm64
make test           # run tests
make clean          # remove dist/
make run            # go run ./cmd/ppdm-simulator
```

### Native build

Build for the current OS and architecture:

```bash
mkdir -p dist
go build -o dist/ppdm-simulator ./cmd/ppdm-simulator
```

### Cross-compilation

Build release binaries for Linux and macOS on `arm64` and `amd64`:

```bash
mkdir -p dist

# Linux amd64
GOOS=linux GOARCH=amd64 go build -o dist/ppdm-simulator-linux-amd64 ./cmd/ppdm-simulator

# Linux arm64
GOOS=linux GOARCH=arm64 go build -o dist/ppdm-simulator-linux-arm64 ./cmd/ppdm-simulator

# macOS amd64 (Intel)
GOOS=darwin GOARCH=amd64 go build -o dist/ppdm-simulator-darwin-amd64 ./cmd/ppdm-simulator

# macOS arm64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o dist/ppdm-simulator-darwin-arm64 ./cmd/ppdm-simulator
```

| Platform | Binary |
|----------|--------|
| Linux x86_64 | `dist/ppdm-simulator-linux-amd64` |
| Linux ARM64 | `dist/ppdm-simulator-linux-arm64` |
| macOS Intel | `dist/ppdm-simulator-darwin-amd64` |
| macOS Apple Silicon | `dist/ppdm-simulator-darwin-arm64` |

Run on the target machine (ensure `openapi-json/` is present in the working directory):

```bash
chmod +x dist/ppdm-simulator-linux-amd64
./dist/ppdm-simulator-linux-amd64
```

On macOS, remove the quarantine flag if the binary was downloaded or copied from another machine:

```bash
xattr -d com.apple.quarantine dist/ppdm-simulator-darwin-arm64 2>/dev/null || true
./dist/ppdm-simulator-darwin-arm64
```

Optional: strip debug symbols for smaller binaries:

```bash
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/ppdm-simulator-linux-amd64 ./cmd/ppdm-simulator
```

The server listens on `https://0.0.0.0:8443` by default (matching the PPDM default port). Only HTTPS is accepted — plain HTTP requests are rejected.

## Authentication

By default, protected endpoints require a bearer token obtained via login:

```bash
# Login (-k skips self-signed certificate verification)
curl -sk -X POST https://localhost:8443/api/v2/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' | jq .

# Use the access_token from the response
curl -sk https://localhost:8443/api/v2/assets \
  -H "Authorization: Bearer <access_token>" | jq .
```

To disable authentication (useful for quick local testing):

```bash
go run ./cmd/ppdm-simulator -no-auth
```

## Examples: login + get assets

Requires [jq](https://jqlang.github.io/jq/) for pretty-printed JSON in shell examples.

Start the simulator first:

```bash
go run ./cmd/ppdm-simulator
```

### 1. Two-step curl

```bash
# Step 1: login
curl -sk -X POST https://localhost:8443/api/v2/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' | jq .

# Step 2: copy access_token from the response, then:
curl -sk 'https://localhost:8443/api/v2/assets?page=1&pageSize=10' \
  -H 'Authorization: Bearer <access_token>' | jq .
```

### 2. One-liner curl (bash)

```bash
TOKEN=$(curl -sk -X POST https://localhost:8443/api/v2/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' \
  | jq -r .access_token) \
&& curl -sk "https://localhost:8443/api/v2/assets?page=1&pageSize=10" \
  -H "Authorization: Bearer ${TOKEN}" | jq .
```

### 3. One-liner curl with jq

```bash
TOKEN=$(curl -sk -X POST https://localhost:8443/api/v2/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' | jq -r .access_token) \
&& curl -sk "https://localhost:8443/api/v2/assets?page=1&pageSize=10" \
  -H "Authorization: Bearer ${TOKEN}" | jq .
```

### 4. Shell script

```bash
./examples/login-get-assets.sh
```

Optional environment variables:

```bash
PPDM_URL=https://localhost:8443 PPDM_USER=admin PPDM_PASSWORD=admin ./examples/login-get-assets.sh
```

### 5. Go client

```bash
go run ./examples/login-get-assets.go
```

With custom settings:

```bash
PPDM_URL=https://localhost:8443 PPDM_USER=admin PPDM_PASSWORD=admin go run ./examples/login-get-assets.go
```

### 6. PowerShell

```powershell
$login = Invoke-RestMethod -SkipCertificateCheck -Method Post `
  -Uri "https://localhost:8443/api/v2/login" `
  -ContentType "application/json" `
  -Body '{"username":"admin","password":"admin"}'

$login | ConvertTo-Json -Depth 10

$assets = Invoke-RestMethod -SkipCertificateCheck -Method Get `
  -Uri "https://localhost:8443/api/v2/assets?page=1&pageSize=10" `
  -Headers @{ Authorization = "Bearer $($login.access_token)" }

$assets | ConvertTo-Json -Depth 10
```

Expected flow in simulator logs:

```
--> POST /api/v2/login op=login ...
<-- POST /api/v2/login 200 ...
--> GET /api/v2/assets op=getAssets auth=Bearer eyJhbGci...
<-- GET /api/v2/assets 200 ...
```


## Options

| Flag | Default | Description |
|------|---------|-------------|
| `-host` | `0.0.0.0` | Bind address |
| `-port` | `8443` | Bind port |
| `-cert` | `./ssl/cert.pem` | TLS certificate file (PEM) |
| `-key` | `./ssl/key.pem` | TLS private key file (PEM) |
| `-ssl-dir` | `ssl` | Directory for stored TLS files |
| `-openapi-dir` | `openapi-json` | Path to OpenAPI JSON files |
| `-no-auth` | off | Skip bearer token validation |
| `-quiet` | off | Disable API request/response logging |

When `-cert` and `-key` are omitted, the simulator loads `cert.pem` and `key.pem` from `./ssl/`. If they are missing, invalid, or expired, a new self-signed certificate is generated.

Managed self-signed certificates:
- **Lifetime:** 7 days
- **Auto-renewal:** checked every hour while the server is running (renews when less than 24 hours remain)
- **Hot reload:** new TLS handshakes use the renewed certificate without restarting the server

Custom certificates provided via `-cert` and `-key` are not auto-renewed.

## Debugging

API calls are logged by default to help troubleshooting:

```
--> POST /api/v2/login op=login auth=-
  {
    "password": "***",
    "username": "admin"
  }
<-- POST /api/v2/login 200 1ms
  {
    "access_token": "***",
    "expires_in": 3600,
    ...
  }
```

Sensitive fields (`password`, tokens) are redacted. Use `-quiet` to disable logging.

## OpenAPI specifications

The simulator reads OpenAPI JSON files from `openapi-json/`. Download them from the [Dell PowerProtect Data Manager REST API](https://developer.dell.com/apis/4378) documentation on [developer.dell.com](https://developer.dell.com).

For **version 20.1.0**, open each module below, then use **Export** to download the OpenAPI JSON file and place it in `openapi-json/`:

| API | Module page | Export file (example) |
|-----|-------------|------------------------|
| v1 (license) | [Module 9627](https://developer.dell.com/apis/4378/versions/20.1.0/modules/9627) | `9627-20.1.0.json` |
| v2 | [Module 9765](https://developer.dell.com/apis/4378/versions/20.1.0/modules/9765) | `9765-20.1.0.json` |
| v3 | [Module 9628](https://developer.dell.com/apis/4378/versions/20.1.0/modules/9628) | `9628-20.1.0.json` |

Steps for each module:

1. Open the module URL (v1, v2, or v3).
2. Click **Export**.
3. Save the JSON file into `openapi-json/` using the naming pattern `<module-id>-<version>.json`.

To use a different PPDM release, select the target version on the [API versions page](https://developer.dell.com/apis/4378), then export the corresponding v1, v2, and v3 modules.

## Covered APIs

| Spec file | API version | Base path | Source |
|-----------|-------------|-----------|--------|
| `9627-20.1.0.json` | v1 (license) | `/dpilm/api/v1/` | [Module 9627](https://developer.dell.com/apis/4378/versions/20.1.0/modules/9627) |
| `9765-20.1.0.json` | v2 | `/api/v2/` | [Module 9765](https://developer.dell.com/apis/4378/versions/20.1.0/modules/9765) |
| `9628-20.1.0.json` | v3 | `/api/v3/` | [Module 9628](https://developer.dell.com/apis/4378/versions/20.1.0/modules/9628) |

## Health check

```bash
curl -sk https://localhost:8443/health | jq .
```

## Project layout

```
cmd/ppdm-simulator/   # CLI entry point
dist/                 # built binaries (gitignored)
examples/             # login + API usage examples
internal/
  auth/               # login / token / logout
  loader/             # OpenAPI spec loader + path matching
  mock/               # schema-based response generator
  server/             # HTTP server
openapi-json/         # PPDM OpenAPI specs
```

## Notes

- Responses are **generated from OpenAPI response schemas** (`responses.{status}.content.application/json.schema`), including required fields, enums, `$ref`, and `allOf`.
- Error responses use the operation's documented schema (typically `ErrorMessage`).
- `POST`, `PUT`, `PATCH`, and `DELETE` return success responses without persisting state.
- Pagination query params (`page`, `pageSize`) are reflected in list responses when applicable.
