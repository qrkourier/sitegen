# sitegen Demo

Welcome to the **sitegen** demo site. This is a static site generated from markdown files using [sitegen](https://github.com/qrkourier/sitegen).

## What is sitegen?

A single-binary static site generator written in Go. It converts markdown files into a browseable HTML site with:

- Sidebar table-of-contents navigation
- GFM table support
- Collapsible directory tree in the sidebar
- Multiple color themes
- Security headers on every response

## Quick start

```bash
go build -o sitegen .
./sitegen build -src content -out docs
./sitegen serve -src content -out docs -addr :8080
```

## How this demo is built

This site is generated automatically by GitHub Actions on every push to `main` and deployed to GitHub Pages. The source markdown lives in the `demo/` directory of the repository.
