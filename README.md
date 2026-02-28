# How to Use sitegen

Static HTML generator written in Go. Converts markdown files to a browseable site with table-of-contents navigation, GFM table support, and security headers.

## Build

Requires Go 1.22+.

```
go build -o sitegen .
```

## Usage

### Generate and exit

```
./sitegen build -src content -out docs
```

- `-src` — markdown source: a directory (walks for `*.md` files) or a single `.md` file
- `-out` — output directory for generated HTML (default: `docs`)

Reads all `.md` files, converts each to HTML with a sidebar TOC, generates an index, and exits.

### Generate, watch, and serve

```
./sitegen serve -src content -out docs -addr :8080
```

- `-src` — markdown source (default: `content`)
- `-out` — output directory (default: `docs`)
- `-addr` — listen address (default: `:8080`)

Performs an initial build, then watches the source directory for changes. When a markdown file is added, removed, or modified, the site is rebuilt automatically. Refresh your browser to see updates.

The server adds security headers to every response: `Content-Security-Policy`, `X-Content-Type-Options`, `X-Frame-Options`, and `Referrer-Policy`.

## OpenZiti

Serve mode can optionally host the site on an [OpenZiti](https://openziti.io/) overlay network, making it accessible only to enrolled Ziti identities without exposing any public ports.

Set two environment variables (both required — Ziti is off if either is unset):

- `ZITI_IDENTITY` — base64-encoded identity JSON (contains controller URL, certs, keys)
- `ZITI_SERVICE` — name of the Ziti service to bind

```
export ZITI_IDENTITY=$(base64 -w0 < identity.json)
export ZITI_SERVICE=my-docs
./sitegen serve
```

When configured, both the TCP listener and the Ziti listener run concurrently. The TCP listener continues to work as usual. Credentials are passed via environment variables to keep them out of process arguments.

If Ziti is misconfigured (bad base64, invalid JSON, wrong service name), an error is logged but the TCP listener continues to function normally.

## ACME TLS

Serve mode can automatically obtain a TLS certificate via ACME DNS-01 challenge using Cloudflare DNS. Set three environment variables (all required — TLS is off if any is unset):

- `DNS_SAN` — domain name for the certificate
- `CLOUDFLARE_API_KEY` — Cloudflare API token with DNS edit permissions
- `TLS_PRIVKEY` — base64-encoded PEM private key for the certificate

```
export DNS_SAN=docs.example.com
export CLOUDFLARE_API_KEY=your-cloudflare-api-token
export TLS_PRIVKEY=$(base64 -w0 < key.pem)
./sitegen serve
```

When configured, the server binds TLS to all active listeners (TCP and Ziti). The certificate is saved to `cert.pem` in the working directory and reused on subsequent starts if it is still valid for the domain and has more than 30 days until expiry.

## Docker

A container image is published to [GitHub Container Registry](https://ghcr.io/qrkourier/sitegen) on every push to `main` and on every release.

Basic usage (plain HTTP):

```
docker run --rm -v ./content:/content:ro -p 8080:8080 \
  ghcr.io/qrkourier/sitegen:latest serve -src /content -out /docs -addr :8080
```

With ACME TLS and/or OpenZiti, pass credentials via `--env-file` and mount `cert.pem` to persist the certificate across restarts. This avoids hitting the ACME issuer's rate limit by reusing a cached certificate. The server automatically renews the certificate during startup if it is within 30 days of expiry.

```
docker run --rm --user $(id -u) \
  --env-file ./.env \
  --volume ./cert.pem:/cert.pem \
  --volume ./content:/content:ro \
  ghcr.io/qrkourier/sitegen:latest serve -src /content -out /docs -addr :8080 -verbose
```

- `--env-file ./.env` — supplies `DNS_SAN`, `CLOUDFLARE_API_KEY`, `TLS_PRIVKEY`, and optionally `ZITI_IDENTITY` and `ZITI_SERVICE` (see sections above)
- `--volume ./cert.pem:/cert.pem` — persists the issued certificate so it is reused on subsequent starts
- `--user $(id -u)` — ensures the container writes `cert.pem` with the host user's ownership
- Replace `-addr :8080` with `-no-addr` to disable the TCP listener and serve exclusively over OpenZiti

### Docker Compose

```
docker compose up
```

Docker Compose loads `.env` automatically, so the environment variables defined there are available without additional configuration. To enable TLS, mount `cert.pem` for certificate persistence. To enable OpenZiti, add the Ziti environment variables to `.env`. Both can be enabled together.

Uncomment the relevant sections in `docker-compose.yml` to enable TLS, OpenZiti, or both.

### Kubernetes

Kustomize-ready manifests are in `deploy/kubernetes/`:

```
kubectl apply -k deploy/kubernetes/
```

This creates a `sitegen` namespace with a Deployment, Service, and Ingress. Edit `deploy/kubernetes/deployment.yml` to configure the image tag, content source, and optional secrets for Ziti or TLS.

To load content into the cluster as a ConfigMap:

```
kubectl create configmap sitegen-content \
  --from-file=content/ -n sitegen
```

## When to restart vs. reload

| Change | Action |
|---|---|
| Edit a markdown file in `content/` | Automatic in serve mode; `sitegen build` in build mode. Reload browser. |
| Add or remove a markdown file | Automatic in serve mode; `sitegen build` in build mode. Reload browser. |
| Edit `static/style.css` or `templates/*.html` | Recompile with `go build`, restart server |
| Change Go source code | Recompile with `go build`, restart server |
| Change `-addr` flag | Restart server |

In serve mode, the file watcher polls every 500ms for changes to markdown files (by modtime and size). The server reads files from disk on each request, so a browser refresh picks up rebuilt content immediately.

Templates and CSS are embedded into the binary via `go:embed`, so changes to files in `templates/` or `static/` require recompiling with `go build`.

## Adding content

Drop any `.md` file into the `content/` directory. In serve mode, the watcher rebuilds automatically. In build mode, re-run `sitegen build`.

```
cp ~/path/to/document.md content/
```

Subdirectories become collapsible sections in the sidebar tree.

## Project structure

```
main.go              CLI entry point (build / serve)
build.go             Markdown-to-HTML pipeline, template rendering
serve.go             HTTP server, file watcher, auto-rebuild
templates/
  page.html          Document page layout with sidebar TOC
  index.html         Index page listing all documents
static/
  style.css          Stylesheet (embedded at compile time)
content/             Markdown source files (not committed)
docs/               Generated output (not committed)
```

## Dependencies

- [goldmark](https://github.com/yuin/goldmark) — markdown parsing with GFM extensions (tables, strikethrough, autolinks, task lists)
- [openziti/sdk-golang](https://github.com/openziti/sdk-golang) — optional OpenZiti overlay network support for serve mode
- [lego](https://github.com/go-acme/lego) — ACME client for automatic TLS certificate provisioning via DNS-01 challenge
