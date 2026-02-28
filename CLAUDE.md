# sitegen

Static site generator in Go. Converts markdown to browsable HTML with sidebar TOC, GFM tables, security headers. Optionally serves over OpenZiti overlay and/or ACME TLS.

## Git policy

Do not use git to add, commit, push, or amend. Do not modify `.git/config` or any remote settings. The user manages all version control operations.

## Stack

- Go (minimum version as specified in `go.mod`), single module, no framework
- goldmark for markdown, lego for ACME, openziti/sdk-golang for Ziti
- pfxlog/logrus for structured logging
- Templates and CSS embedded via `go:embed`

## Commands

```bash
go build -o sitegen .                # build binary
go test ./...                        # run all tests
go vet ./...                         # static analysis
```

## Key files

| File | Purpose |
|---|---|
| `main.go` | CLI entry point: `build` and `serve` subcommands |
| `build.go` | Markdown-to-HTML pipeline, template rendering |
| `serve.go` | HTTP server, file watcher, auto-rebuild |
| `tls.go` | ACME certificate provisioning (DNS-01 via Cloudflare) |
| `templates/` | HTML templates (embedded at compile time) |
| `static/` | CSS and JS (embedded at compile time) |
| `docker-compose.yml` | Compose config; `.env` loaded automatically |
| `deploy/kubernetes/` | Kustomize manifests |

## Environment variables

All optional. TLS requires all three `DNS_SAN`/`CLOUDFLARE_API_KEY`/`TLS_PRIVKEY`. Ziti requires both `ZITI_IDENTITY`/`ZITI_SERVICE`.

- `DNS_SAN` — domain name for the ACME certificate
- `CLOUDFLARE_API_KEY` — Cloudflare API token (DNS edit)
- `TLS_PRIVKEY` — base64-encoded PEM private key
- `ZITI_IDENTITY` — base64-encoded identity JSON
- `ZITI_SERVICE` — Ziti service name to bind

## Conventions

- No CGO; binary is statically linked (`CGO_ENABLED=0`)
- Errors are returned, not panicked; log with pfxlog
- Templates and static assets require recompilation after changes
- `cert.pem` is written to the working directory and reused if valid (>30 days to expiry)
- The `.env` file contains secrets — never commit it
