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
