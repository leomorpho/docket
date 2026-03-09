# TASK-004: Backend interface + local markdown store

## Status
`[ ]` not started

## Depends on
TASK-003

## What this is
Two things in one task:
1. Define the `Backend` interface — the contract all storage backends must fulfill
2. Implement the local markdown backend (reads/writes `.docket/tickets/TKT-NNN.md` files)

This is the core persistence layer. Future backends (JIRA, Linear, Confluence) implement
the same interface without touching any CLI code.

## Why a Backend interface?

The CLI never imports a concrete store directly — it always works through `store.Backend`.
Today's implementation is `store/local`. Future implementations drop in cleanly:

```
internal/store/
  backend.go         ← Backend interface + Filter type
  local/
    store.go         ← LocalMarkdown implementation
    parser.go        ← markdown/YAML parser
    writer.go        ← markdown file writer
  jira/
    store.go         ← (future) JIRA REST API implementation
  linear/
    store.go         ← (future) Linear GraphQL implementation
```

Config's `"backend": "local"` field determines which implementation is used at runtime.

## Files to create

### `internal/store/backend.go`

```go
package store

import (
    "context"
    "github.com/leoaudibert/docket/internal/ticket"
)

// Filter specifies which tickets to return from ListTickets.
type Filter struct {
    States          []ticket.State // empty = all non-archived
    Labels          []string       // empty = no filter
    MaxPriority     int            // 0 = no filter; N = only priority <= N
    OnlyUnblocked   bool
    IncludeArchived bool
}

// Backend is the interface all storage implementations must satisfy.
// The local markdown store is the default. Future: JIRA, Linear, Confluence.
type Backend interface {
    // CreateTicket persists a new ticket. ID and Seq must already be set.
    CreateTicket(ctx context.Context, t *ticket.Ticket) error

    // UpdateTicket overwrites an existing ticket's mutable fields.
    // Implementations must preserve comments and append-only sections.
    UpdateTicket(ctx context.Context, t *ticket.Ticket) error

    // GetTicket returns a single ticket by ID. Returns nil, nil if not found.
    GetTicket(ctx context.Context, id string) (*ticket.Ticket, error)

    // ListTickets returns tickets matching the filter, sorted by priority then created_at.
    ListTickets(ctx context.Context, f Filter) ([]*ticket.Ticket, error)

    // AddComment appends a comment to a ticket. Never edits existing content.
    AddComment(ctx context.Context, id string, c ticket.Comment) error

    // LinkCommit appends a commit SHA to a ticket's linked_commits.
    LinkCommit(ctx context.Context, id string, sha string) error

    // NextID generates the next sequential ticket ID. Mutates the counter.
    NextID(ctx context.Context) (id string, seq int, err error)

    // Validate checks whether a ticket is schema-valid for this backend.
    // For local: checks markdown structure. For remote: checks API constraints.
    Validate(ctx context.Context, id string) ([]ValidationError, error)
}

// ValidationError describes a single schema violation found in a ticket.
type ValidationError struct {
    Field   string // e.g. "state", "blocked_by[0]", "frontmatter"
    Message string // human-readable description
}
```

### `internal/store/local/parser.go`

Parse a `.docket/tickets/TKT-NNN.md` file into a `ticket.Ticket`.

The markdown format is:
```
---
<YAML frontmatter>
---

# TKT-001: Title here

## Description
...

## Acceptance Criteria
- [ ] item one
- [x] item two — evidence: some text

## Plan
1. [done] step one
2. [pending] step two

## Comments

### 2026-03-09T14:30:00Z — human:leoaudibert
comment body

## Handoff
handoff body
```

Implementation notes:
- Split on first `---` delimiter to extract YAML frontmatter
- Parse frontmatter with `gopkg.in/yaml.v3` into the struct's frontmatter fields
- Parse `# TKT-NNN: Title` from the first H1 to extract title
- Parse each `## Section` into the corresponding struct field
- AC lines: `- [ ] description` → `Done: false`, `- [x] description — evidence: X` → `Done: true, Evidence: X`
- Plan lines: `N. [done] description` → `Status: done`, `N. [pending] description` → `Status: pending`
- Comments: each `### TIMESTAMP — ACTOR` block → `ticket.Comment{At, Author, Body}`
- If a section is missing, leave the field empty — do not error

### `internal/store/local/writer.go`

Render a `ticket.Ticket` back to markdown. Must be the inverse of parser.

Key constraints:
- Comments are NEVER rewritten — only appended via `AddComment`
- `UpdateTicket` rewrites frontmatter + Description + AC + Plan + Handoff, then re-appends existing comments
- The rendered output must parse back to the same struct (round-trip stable)

### `internal/store/local/store.go`

Implement `store.Backend` using the parser and writer.

```go
package local

type Store struct {
    RepoRoot string
}

func New(repoRoot string) *Store { return &Store{RepoRoot: repoRoot} }

func (s *Store) ticketPath(id string) string {
    return filepath.Join(s.RepoRoot, ".docket", "tickets", id+".md")
}
```

- `CreateTicket`: render to markdown, write file (fail if already exists)
- `UpdateTicket`: parse existing file to get comments, rewrite with updated fields + original comments appended
- `GetTicket`: parse the file, return nil,nil if file not found
- `ListTickets`: glob `*.md` files, parse each, apply filter, sort
- `AddComment`: append a `### TIMESTAMP — ACTOR\n\nbody\n` block to the `## Comments` section
- `LinkCommit`: parse → add SHA to `linked_commits` → write back via `UpdateTicket`
- `NextID`: delegates to `ticket.NextID(s.RepoRoot)`
- `Validate`: covered in TASK-005, stub returning nil for now

## Dependencies to add
```bash
go get gopkg.in/yaml.v3
```

## Acceptance criteria
- [ ] `CreateTicket` writes a valid markdown file at the correct path
- [ ] `GetTicket` on a missing ID returns `nil, nil`
- [ ] `GetTicket` after `CreateTicket` returns the same data (round-trip)
- [ ] `UpdateTicket` preserves existing comments
- [ ] `AddComment` appends to the `## Comments` section without touching other content
- [ ] `ListTickets` with `Filter{States: []State{StateInProgress}}` returns only in-progress tickets
- [ ] `ListTickets` sorts by priority ascending, then created_at ascending
- [ ] `LinkCommit` adds the SHA to `linked_commits` in frontmatter
- [ ] `go test ./internal/store/local/...` passes using `t.TempDir()`

## Notes for LLM
- Do NOT implement validation logic here — that is TASK-005
- Round-trip stability is critical: `parse(render(t)) == t` for all fields
- Use `os.MkdirAll` to create the `tickets/` directory on first write
- Tests should cover the edge case of a ticket with no optional sections
