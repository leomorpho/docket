# Architecture

Design decisions for `docket`, recorded so future contributors (human or LLM) understand
why things are the way they are.

---

## Storage: Markdown with YAML frontmatter

Each ticket is a single `.md` file at `.docket/tickets/TKT-001.md`.

**Why not JSONL (append-only event log)?**
JSONL is more "correct" engineering — append-only, deterministic replay. But every read
requires the CLI. An LLM or human cannot open the file and understand the ticket without tooling.
The format discourages direct editing, but it also discourages direct reading.

**Why not SQLite as source of truth?**
Binary file. Useless git diffs. Merge conflicts that can't be resolved textually.

**Why markdown + YAML frontmatter?**
- Human-readable in any editor, on GitHub, everywhere
- LLMs can read ticket files directly without the CLI
- Git diffs are meaningful — state changes, new comments, updated AC are all visible
- Anyone can write (human, LLM, CLI binary) — we validate at the gate, not at write time

**Who can write ticket files?**
Anyone. The CLI is a convenience, not a gatekeeper for writes. What matters is that what
lands in git is schema-valid. `docket validate` enforces this at pre-commit time.

---

## Validation at the gate, not at write time

This is the key design decision. Rather than cryptographic checksums or file locking,
we validate schema compliance when it matters: before a commit lands.

- `docket validate TKT-001` — validate one file
- `docket validate` — validate all ticket files
- Pre-commit hook (lefthook) — runs validate on any staged `.docket/tickets/*.md` files
- `docket check` — full consistency check (staleness, broken references, etc.)

What "valid" means:
- All required frontmatter fields present
- `state` is one of the valid values
- `id` matches filename (`TKT-001.md` must contain `id: TKT-001`)
- `priority` is a positive integer
- `blocked_by` references actually exist as ticket files
- Dates parse as RFC3339
- Required markdown sections present (`## Description`, `## Acceptance Criteria`)

Invalid changes are caught at commit time, not silently accepted.

---

## Ticket file format

```markdown
---
id: TKT-001
seq: 1
state: in-progress
priority: 1
labels:
  - feature
  - llm-only
blocked_by: []
blocks: []
linked_commits:
  - abc123def456
created_at: 2026-03-09T14:22:00Z
updated_at: 2026-03-09T15:00:00Z
created_by: human:leoaudibert
---

# TKT-001: Add auth middleware

## Description

We need JWT middleware before the rate limiter can work correctly.
Blocked by the rate limiting ticket because order of middleware matters.

## Acceptance Criteria

- [ ] Middleware validates tokens on all /api/* routes
- [ ] Invalid token returns 401 with structured error body
- [x] Unit tests cover happy path and 3 error cases — evidence: `go test ./...` passed 2026-03-09

## Plan

1. [done] Research JWT library options
2. [pending] Implement middleware
3. [pending] Wire into router in cmd/server.go

## Comments

### 2026-03-09T14:30:00Z — human:leoaudibert

Decided against `golang-jwt/jwt` in favor of `lestrrat-go/jwx` — better key rotation support.

### 2026-03-09T15:00:00Z — agent:claude-sonnet-4-6

Implemented token validation. Tests pass. Router wiring is next step.

## Handoff

*Last updated: 2026-03-09T15:00:00Z by agent:claude-sonnet-4-6*

**Current state:** Token validation logic is done. Router wiring remains.

**Decisions made:** Used lestrrat-go/jwx for key rotation (see comment 2026-03-09T14:30).

**Files touched:** `internal/auth/middleware.go`, `internal/auth/middleware_test.go`

**Remaining work:** Wire middleware into router in `cmd/server.go`. Update integration tests.

**AC status:** 1/3 complete.
```

Comments are always appended, never edited. The Handoff section is replaced on each
`docket session compress` call.

---

## SQLite cache

`.docket/index.db` is derived by parsing all ticket markdown files. Always gitignored.
Rebuilt on demand (`docket sync` or automatically when the CLI detects it's stale).

Used for fast queries: `--state`, `--label`, `--unblocked`, priority sorting, staleness checks.
Reading a single ticket (`docket show`) goes directly to the markdown file.

---

## Sequential IDs

Counter in `.docket/config.json`. Sequential (`TKT-001`, `TKT-002`) — human-readable,
sortable, tells you how many tickets exist. For solo/small-team use, concurrent creation
conflicts on `config.json` are acceptable (resolve during merge).

`blocked` is not a state — it is computed from `blocked_by` being non-empty with tickets
that are not `done`/`archived`. The kanban board renders it as a separate computed column.

---

## CLI + MCP dual surface

One binary. `docket serve --mcp` starts an MCP server that calls the same `internal/`
library functions as the CLI. No schema drift between surfaces.

`docket help-json` outputs a machine-readable command manifest for LLM shell usage.

---

## Git integration

**Commit trailers:** `Ticket: TKT-001` in commit message body.
`docket blame file:line` wraps `git blame`, finds commit SHA, reads trailer, returns ticket.

**Inline annotations:** `// [TKT-001] reason` in source code.
`docket scan` indexes these. `docket refs TKT-001` shows all locations.

**Pre-commit hook (lefthook):**
- `docket validate` on staged ticket files (schema check)
- `docket ac check TKT-001` if commit message contains `Ticket: TKT-001` (AC gate)

---

## Handoff summary

Written by LLM at end of session via `docket session compress TKT-001`.
Replaces the `## Handoff` section in the markdown file.
Required sections validated by `docket validate`:
- Current state
- Decisions made
- Files touched
- Remaining work
- AC status

---

## Dependencies

- `github.com/spf13/cobra` — CLI
- `github.com/charmbracelet/bubbletea` — TUI kanban board
- `github.com/charmbracelet/lipgloss` — TUI styling
- `modernc.org/sqlite` — SQLite cache (pure Go, no CGO)
- `gopkg.in/yaml.v3` — YAML frontmatter parsing
