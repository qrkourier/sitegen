# Markdown Features

sitegen uses [goldmark](https://github.com/yuin/goldmark) with GitHub Flavored Markdown extensions. This page demonstrates the supported features.

## Tables

| Feature | Supported | Notes |
|---|---|---|
| GFM tables | Yes | Striped rows, responsive layout |
| Task lists | Yes | Rendered as checkboxes |
| Strikethrough | Yes | `~~text~~` syntax |
| Autolinks | Yes | URLs become clickable |
| Heading IDs | Yes | Auto-generated for TOC anchors |

## Code blocks

Fenced code blocks with language hints:

```go
package main

import "fmt"

func main() {
    fmt.Println("Hello from sitegen!")
}
```

Inline code like `./sitegen build` is also supported.

## Task lists

- [x] Markdown parsing
- [x] Table of contents
- [x] Sidebar navigation
- [x] Multiple themes
- [ ] Search (not yet)

## Blockquotes

> sitegen embeds templates and CSS into the binary at compile time.
> Change a template or stylesheet, recompile, and the update ships with the binary.

## Nested lists

1. Build modes
   - `build` — generate and exit
   - `serve` — generate, watch, and serve
2. Network listeners
   - TCP (plain or ACME TLS)
   - OpenZiti overlay
3. Deployment targets
   - Docker / Docker Compose
   - Kubernetes (Kustomize)
   - GitHub Pages

## Links and emphasis

Visit the [repository](https://github.com/qrkourier/sitegen) for the source code. Text can be **bold**, *italic*, or ~~struck through~~.
