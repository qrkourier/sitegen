# Getting Started

This guide walks through building and running sitegen locally.

## Prerequisites

- Go 1.22 or later
- A directory of `.md` files to convert

## Build the binary

```bash
go build -o sitegen .
```

The binary is statically linked (no CGO) and has no runtime dependencies.

## Generate a site

```bash
mkdir -p content
echo "# Hello World" > content/hello.md
./sitegen build -src content -out docs
```

This creates `docs/` with:

| File | Purpose |
|---|---|
| `index.html` | Listing of all pages with sort controls |
| `hello.html` | Rendered markdown page with TOC sidebar |
| `static/style.css` | Stylesheet with theme support |
| `static/theme.js` | Theme persistence via localStorage |

## Serve locally

```bash
./sitegen serve -src content -out docs -addr :8080
```

Open `http://localhost:8080` in a browser. Edit any `.md` file and refresh — the file watcher rebuilds automatically.

## Serve anywhere with overlays

sitegen can serve your site over [OpenZiti](https://openziti.io/) or [zrok](https://zrok.io/) overlay networks, making it accessible from anywhere without a VPS or public IP.

When overlay environment variables are configured, the TCP listener is suppressed by default — the site is only reachable through the overlay. Pass `-addr :8080` explicitly if you also want a local TCP listener.

### zrok

With a [zrok account](https://zrok.io/) and `zrok enable` run once:

```bash
ZROK2_ENABLE_TOKEN=your-token ./sitegen serve -src content -out docs
```

sitegen creates a public zrok share and prints the access URL at startup.

### OpenZiti

```bash
export ZITI_IDENTITY=$(base64 -w0 < identity.json)
export ZITI_SERVICE=my-docs
./sitegen serve -src content -out docs
```

## Directory structure

Subdirectories in the source become collapsible sections in the sidebar tree:

```
content/
  README.md
  guides/
    getting-started.md
    deployment.md
  reference/
    api.md
```
