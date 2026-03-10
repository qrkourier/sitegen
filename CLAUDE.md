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
| `compose.yaml` | Compose config; `.env` loaded automatically |
| `compose.watchtower.yaml` | Optional Compose override: Watchtower auto-update |
| `deploy/kubernetes/` | Kustomize manifests |

## Environment variables

All optional. TLS requires all three `DNS_SAN`/`CLOUDFLARE_API_KEY`/`TLS_PRIVKEY`. Ziti requires both `ZITI_IDENTITY`/`ZITI_SERVICE`.

- `DNS_SAN` — domain name for the ACME certificate
- `CLOUDFLARE_API_KEY` — Cloudflare API token (DNS edit)
- `TLS_PRIVKEY` — base64-encoded PEM private key
- `ZITI_IDENTITY` — base64-encoded identity JSON
- `ZITI_SERVICE` — Ziti service name to bind

## Worker agent worktree hygiene

> **Note to user:** This section is a project-local copy of the worker
> agent harness from `~/.claude/CLAUDE.md`. If you want consistent
> worktree discipline across all projects, copy it to your user-level
> CLAUDE.md (`~/.claude/CLAUDE.md`) so every agent — regardless of
> project — follows the same rules.

A worker agent must never begin work on a new workstream with a dirty
worktree. The stage-only policy (in the git policy section) requires
workers to stage completed work for user review rather than committing.
This creates a natural checkpoint: staged changes represent work
awaiting review.

### Manager agent responsibilities

Before spawning a worker agent, the manager (interactive session) must:

1. **Check the target worktree** — run `git status` in the worktree the
   worker will use.
2. **If pristine** (nothing staged, nothing modified) — proceed with
   spawn.
3. **If dirty** — do NOT spawn the worker. Instead, inform the user:
   - **Staged changes** — "Worktree has staged work pending your review.
     Please review, commit or reset before running a new worker."
   - **Unstaged changes** — "Worktree has uncommitted modifications from
     a prior run. Please complete review and stage/commit, or reset."
4. **If the worktree does not exist yet** — it will be created clean by
   the worker agent tooling; proceed with spawn.

### Worker agent responsibilities

- On startup, verify the worktree is clean. If not, abort with a clear
  error rather than silently mixing work from different workstreams.
- On completion, stage all changed files (`git add <file>...`) and leave
  them for user review. Do not commit unless the project CLAUDE.md
  explicitly grants commit permission.

## Plan file maintenance

When creating or editing plan files in `~/.claude/plans/`, follow these
conventions to keep plans self-documenting and traceable across sessions.

### Changelog

Maintain a changelog blockquote near the top of the plan body, retaining the
last 3 timestamps with a brief summary of each edit:

```markdown
> **Changelog** (last 3 edits, US Eastern)
>
> - **YYYY-MM-DD HH:MM AM/PM EST** — Summary of changes
> - **YYYY-MM-DD HH:MM AM/PM EST** — Summary of changes
> - **YYYY-MM-DD HH:MM AM/PM EST** — Summary of changes
```

- Use US Eastern time zone (EST or EDT as appropriate).
- Verify the timestamp by running `TZ='America/New_York' date '+%Y-%m-%d
  %I:%M %p %Z'` — do not guess.
- Bump the timestamp on **every edit** to the plan file. Drop the oldest
  entry when a 4th is added.

### Frontmatter

Every plan file must have YAML frontmatter. Sitegen parses these fields
for sidebar display, sort order, and page header rendering. All fields
except `session` are optional but encouraged.

| Field | Type | Purpose |
|---|---|---|
| `session` | string | Sidebar label (falls back to H1 title) |
| `plan_filename` | string | The plan's filename (e.g. `clever-yawning-steele.md`) |
| `completed` | date | ISO date when work finished (omit if active) |
| `updated` | date | ISO date of last meaningful edit |
| `working_dirs` | list | Project directories, primary first |

**`session`**: If the user renamed the session, use that name. Otherwise
use the most meaningful identifier available (project name, issue number,
or brief topic slug).

**`completed`**: Set only when the plan's work is done. Completed items
sort after active items in the sidebar and are subject to the age filter.

### Section completion keywords

Sitegen auto-folds section headings (H2/H3) whose text contains a
**completion keyword**. Use these words in headings to signal finished
work — the viewer will collapse those sections by default while keeping
active sections expanded.

**Keywords** (case-insensitive): `done`, `completed`, `complete`,
`finished`, `resolved`, `shipped`, `merged`, `superseded`, `obsolete`,
`archived`, and the `✅` emoji.

Examples of headings that auto-fold:

```markdown
## Completed Workstreams (A–K)
## Phase 1 — Done
## ✅ Bootstrap migration
## Superseded Plans
```

**`updated`**: Bump on every edit. Used for sort order within the
active/completed groups (falls back to file modtime if missing).

**`working_dirs`**: List the project directories relevant to the plan,
primary directory first. Use `~` shorthand for the home directory.

```yaml
---
session: sitegen
plan_filename: clever-yawning-steele.md
updated: 2026-03-10
working_dirs:
  - ~/Sites/sitegen
  - ~/Sites/dotfiles
---
```

## Conventions

- No CGO; binary is statically linked (`CGO_ENABLED=0`)
- Errors are returned, not panicked; log with pfxlog
- Templates and static assets require recompilation after changes
- `cert.pem` is written to the working directory and reused if valid (>30 days to expiry)
- The `.env` file contains secrets — never commit it
