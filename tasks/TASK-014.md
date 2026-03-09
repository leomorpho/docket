# TASK-014: Git blame integration

## Status
`[ ]` not started

## Depends on
TASK-005 (store and config available)

## What this is
`docket blame <file>:<line>` wraps `git blame` to find which ticket a line of code belongs to.
Chains: line → commit SHA → `Ticket: TKT-NNN` trailer → full ticket context.

## Convention: commit trailers
Every commit touching docket-tracked work should include a trailer line:
```
Ticket: TKT-001
```

Git trailers are lines in the commit message body formatted as `Key: Value`.
They are queryable via `git log --format=%(trailers:key=Ticket,valueonly)`.

## Command signature
```
docket blame <file>:<line>
```

Example: `docket blame internal/auth/middleware.go:42`

## Behavior
1. Run `git blame -L <line>,<line> --porcelain <file>` in the repo root
2. Parse the commit SHA from the blame output
3. Run `git log -1 --format=%(trailers:key=Ticket,valueonly) <sha>` to get the ticket ID
4. If no trailer found: print "(no ticket linked to this commit)" and exit 0
5. If ticket found: call `backend.GetTicket(ctx, id)` and print full context

## Output

### human (ticket found)
```
TKT-001 · in-progress · P1 — Add auth middleware

  This line was last modified in commit abc123def (2026-03-09)
  Ticket: TKT-001

  Handoff: Token validation done. Router wiring remains.
  AC: 1/3 done.
```

### human (no ticket)
```
No ticket linked to commit abc123def.
Commit: "refactor: clean up middleware imports" (2026-03-09, human:leoaudibert)
```

### json
```json
{
  "file": "internal/auth/middleware.go",
  "line": 42,
  "commit": "abc123def456",
  "ticket_id": "TKT-001",
  "ticket": { ...full ticket object... }
}
```

## Files to create

### `internal/git/blame.go`
```go
package git

// BlameResult holds the result of blaming a single line.
type BlameResult struct {
    SHA     string
    Author  string
    Date    string
    Summary string
}

// BlameLine runs git blame on a single line and returns commit info.
func BlameLine(repoRoot, file string, line int) (*BlameResult, error)

// CommitTicket returns the ticket ID from a commit's Ticket: trailer, or "" if none.
func CommitTicket(repoRoot, sha string) (string, error)
```

### `cmd/blame.go`
Parse `<file>:<line>` argument, call `git.BlameLine`, call `git.CommitTicket`,
fetch and display ticket if found.

## Acceptance criteria
- [ ] `docket blame main.go:1` on a committed file returns blame info
- [ ] `docket blame main.go:1` on a file with a `Ticket:` trailer shows the ticket
- [ ] `docket blame main.go:1` on a commit without a trailer prints "(no ticket linked)"
- [ ] `docket blame nonexistent.go:1` exits 1 with clear error
- [ ] `docket blame main.go:99999` (line out of range) exits 1 with clear error
- [ ] `--format json` outputs the full JSON structure
- [ ] `go test ./internal/git/...` passes (use a temp git repo in tests)

## Notes for LLM
- `git blame --porcelain` outputs structured data — parse the commit SHA from the first line
- The `%(trailers:...)` format requires git 2.18+; add a version check or graceful fallback
- Tests need a real git repo with commits — use `os/exec` to run `git init`, `git commit` in `t.TempDir()`
