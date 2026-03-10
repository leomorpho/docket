# TASK-016: `docket context` command

## Status
`[x]` done

## Depends on
TASK-012 (SQLite index), TASK-015 (annotation index)

## What this is
`docket context <file>` gives an LLM everything it needs to understand why a file looks
the way it does. Combines blame-based ticket lookup with inline annotation lookup.

## Command signature
```
docket context <file> [--lines <start>-<end>] [--format human|json|context]
```

## Behavior
1. Run `git blame --porcelain <file>` to get all commits that touched the file
2. For each unique commit SHA: look up `Ticket:` trailer → fetch ticket summary
3. Query annotation index for `[TKT-NNN]` references in this file
4. Deduplicate tickets, sort by most recently touched line
5. Output combined context

## Human output
```
Context for internal/auth/middleware.go

Tickets from git history:
  TKT-001 (in-progress, P1) — Add auth middleware
    Lines: 1-89 (most recently touched)
    AC: 1/3 done. Handoff: Token validation done, router wiring remains.

  TKT-000 (done) — Project initialization
    Lines: 1-15 (initial scaffold)

Inline annotations:
  Line 42: [TKT-001] JWT validation must happen before rate limiting
  Line 67: [TKT-001] Middleware order is significant

Linked tickets: TKT-001, TKT-000
```

## Context format (LLM-optimized)
Designed to fit into an LLM prompt efficiently:
```
FILE: internal/auth/middleware.go
TICKETS:
  TKT-001 in-progress P1 | Add auth middleware
    HANDOFF: Token validation done. Router wiring remains.
    AC: 1/3. REMAINING: [validate /api/* tokens] [401 on invalid]
    COMMITS: abc123(line 42), def456(line 1)
  TKT-000 done | Project initialization
    COMMITS: aaa000(line 1)
ANNOTATIONS:
  L42: [TKT-001] JWT validation must happen before rate limiting
  L67: [TKT-001] Middleware order is significant
```

## JSON output
```json
{
  "file": "internal/auth/middleware.go",
  "tickets": [
    {
      "id": "TKT-001",
      "title": "Add auth middleware",
      "state": "in-progress",
      "handoff": "...",
      "ac_status": {"total": 3, "done": 1},
      "lines_touched": [1, 42, 67, 89]
    }
  ],
  "annotations": [
    {"line": 42, "ticket_id": "TKT-001", "context": "JWT validation must happen before rate limiting"}
  ]
}
```

## File to create
`cmd/context.go`

## Acceptance criteria
- [x] `docket context main.go` returns ticket context from git history
- [x] `docket context main.go` includes inline annotations from scan index
- [x] Files with no ticket history print "No tickets linked to this file's history"
- [x] `--lines 40-50` limits blame to that line range
- [x] `--format context` outputs LLM-optimized compact format
- [x] `--format json` outputs valid JSON
- [x] `go test ./cmd/...` passes

## Notes for LLM
- This is one of the highest-value commands for LLMs — optimize the context format for token efficiency
- If the annotation index is empty (never scanned), skip that section gracefully
- `git blame --porcelain` on a large file is slow — consider caching or limiting to `--lines` by default
