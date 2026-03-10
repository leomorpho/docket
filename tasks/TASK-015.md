# TASK-015: Inline annotation scan + refs

## Status
`[x]` done

## Depends on
TASK-014

## What this is
Index `// [TKT-001]` style inline annotations in source code. Complements git blame —
blame is automatic (requires no code changes), annotations are explicit markers for
architecturally significant decisions.

## Annotation format
Any line containing `[TKT-NNN]` (square brackets, uppercase TKT, digits) is an annotation.
Works in any file type — comments in Go, JS, Python, shell, markdown, etc.

```go
// [TKT-042] Rate limiting workaround — JWT expiry check must happen before rate check
func validateRequest(r *http.Request) error {
```

```python
# [TKT-007] This O(n²) loop is intentional — n is always < 20 in practice
for item in items:
```

## Commands

### `docket scan`
```
docket scan [--path <dir>]
```
Walks the repo (excluding `.docket/`, `.git/`, `vendor/`, `node_modules/`),
finds all `[TKT-NNN]` patterns, stores results in SQLite index.

Output:
```
Scanned 142 files. Found 7 annotations across 3 tickets.
```

### `docket refs <TKT-NNN>`
```
docket refs TKT-001
```
Shows all source locations referencing TKT-001.

Output:
```
TKT-001 referenced in 3 locations:

  internal/auth/middleware.go:42
    // [TKT-001] JWT validation must happen before rate limiting

  internal/auth/middleware_test.go:18
    // [TKT-001] Test covers the edge case documented in the ticket

  cmd/server.go:67
    // [TKT-001] Middleware order is significant — see ticket for rationale
```

## SQLite schema additions (add to TASK-012's index)
```sql
CREATE TABLE annotations (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id  TEXT NOT NULL,
    file_path  TEXT NOT NULL,
    line_num   INTEGER NOT NULL,
    context    TEXT NOT NULL  -- the annotation line content
);
CREATE INDEX idx_annotations_ticket ON annotations(ticket_id);
```

## Files to create

### `internal/git/scan.go`
```go
package git

type Annotation struct {
    TicketID string
    FilePath string
    LineNum  int
    Context  string
}

// ScanAnnotations walks dir and returns all [TKT-NNN] annotations found.
func ScanAnnotations(repoRoot string) ([]Annotation, error)
```

### `cmd/scan.go`
Calls `git.ScanAnnotations`, stores results in SQLite via `store.UpsertAnnotations`.

### `cmd/refs.go`
Queries SQLite for annotations matching a ticket ID.

## Acceptance criteria
- [x] `docket scan` finds `// [TKT-001]` in a Go file and stores it
- [x] `docket scan` ignores `.docket/`, `.git/`, `vendor/`, `node_modules/`
- [x] `docket refs TKT-001` shows all file:line locations
- [x] `docket refs TKT-999` on unknown ticket prints "No annotations found" (not an error)
- [x] Running `docket scan` twice doesn't duplicate entries (upsert, not insert)
- [x] Annotations in Python (`# [TKT-001]`), JS (`// [TKT-001]`), shell (`# [TKT-001]`) all found
- [x] `go test ./internal/git/...` passes

## Notes for LLM
- Use `filepath.WalkDir` to traverse files; skip binary files (check for null bytes in first 512 bytes)
- The regex for annotation detection: `\[TKT-\d+\]`
- `docket scan` should be fast enough to run in a pre-commit hook if needed
