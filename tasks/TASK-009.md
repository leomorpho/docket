# TASK-009: `docket show` command

## Status
`[ ]` not started

## Depends on
TASK-006

## Command signature
```
docket show <TKT-NNN> [--format json|md|human|context]
```

## Formats

### human (default)
```
TKT-001 · in-progress · P1 · feature, llm-only

  Add auth middleware

  Description
  ───────────
  We need JWT middleware before the rate limiter can work correctly.

  Acceptance Criteria
  ───────────────────
  [ ] Middleware validates tokens on all /api/* routes
  [ ] Invalid token returns 401 with structured error body
  [x] Unit tests cover happy path — evidence: go test ./... passed 2026-03-09

  Plan
  ────
  1. [done]    Research JWT library options
  2. [pending] Implement middleware
  3. [pending] Wire into router

  Comments (2)
  ────────────
  2026-03-09T14:30:00Z — human:leoaudibert
    Decided against golang-jwt/jwt in favor of lestrrat-go/jwx.

  2026-03-09T15:00:00Z — agent:claude-sonnet-4-6
    Implemented token validation. Tests pass. Router wiring is next step.

  Handoff
  ───────
  Current state: Token validation done. Router wiring remains.
  ...

  Linked commits: abc123def456
  Blocked by: (none)
```

### md
Raw markdown file content as-is.

### json
Full ticket struct as JSON including all fields.

### context (LLM-optimized)
Compact representation prioritizing information density. Includes description, AC status,
plan, handoff, and relevant files. Used when an LLM needs full context on a ticket to
continue work:
```
TICKET: TKT-001 · in-progress · P1
TITLE: Add auth middleware
DESCRIPTION: We need JWT middleware before the rate limiter...
AC: 1/3 done. Remaining: [validate tokens on /api/*] [401 on invalid token]
PLAN: done:[research JWT] pending:[implement middleware] pending:[wire into router]
HANDOFF: Token validation done. Router wiring remains. Files: internal/auth/middleware.go
LINKED COMMITS: abc123def456
BLOCKED BY: none
```

## File to create
`cmd/show.go`

## Acceptance criteria
- [ ] `docket show TKT-001` prints human-readable output
- [ ] `docket show TKT-001 --format md` prints raw markdown file content
- [ ] `docket show TKT-001 --format json` outputs valid JSON with all fields
- [ ] `docket show TKT-001 --format context` outputs compact LLM-optimized format
- [ ] `docket show TKT-999` (nonexistent) exits 1 with clear "not found" message
- [ ] `go test ./cmd/...` passes
