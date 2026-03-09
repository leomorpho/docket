# TASK-005: Schema validator (`docket validate`)

## Status
`[ ]` not started

## Depends on
TASK-004

## What this is
Implement the schema validator and the `docket validate` CLI command. This is the
enforcement mechanism: anyone can write ticket files, but invalid changes are caught
before they land in git. See ARCHITECTURE.md ("Validation at the gate").

## Validation rules for local markdown tickets

A ticket file is valid if ALL of the following hold:

| Rule | Field | Check |
|------|-------|-------|
| Required | `id` | present and matches `TKT-\d+` |
| Required | `seq` | present and > 0 |
| Required | `state` | one of the valid states (not "blocked") |
| Required | `priority` | integer > 0 |
| Required | `created_at` | parseable as RFC3339 |
| Required | `updated_at` | parseable as RFC3339 |
| Required | `created_by` | non-empty string |
| Consistency | `id` vs filename | `TKT-001.md` must contain `id: TKT-001` |
| References | `blocked_by` | each ID must exist as a ticket file |
| References | `blocks` | each ID must exist as a ticket file |
| Structure | markdown body | `## Description` section present |
| Structure | markdown body | `## Acceptance Criteria` section present |
| No cycles | `blocked_by` | ticket must not directly or transitively block itself |

Warnings (non-fatal, reported but don't fail):
- `priority` not set (defaulted to 10)
- `labels` is empty
- `## Handoff` section missing (recommended but not required)

## Files to create

### `internal/store/local/validate.go`

```go
package local

import (
    "github.com/leoaudibert/docket/internal/store"
    "github.com/leoaudibert/docket/internal/ticket"
)

// ValidateFile validates a single ticket markdown file by path.
// Returns a list of errors (blocking) and warnings (non-blocking).
func ValidateFile(repoRoot, filePath string) (errs []store.ValidationError, warns []store.ValidationError, err error)

// ValidateAll validates all tickets in .docket/tickets/.
// Returns a map of ticketID → errors.
func ValidateAll(repoRoot string) (map[string][]store.ValidationError, error)

// detectCycles checks for circular blocked_by dependencies using DFS.
// Returns an error if any cycle is found.
func detectCycles(repoRoot string) error
```

Implement `Validate` on `local.Store` to call `ValidateFile`.

### `cmd/validate.go`

```
docket validate [TKT-NNN] [--warn]
```

Flags:
- `TKT-NNN` (optional positional): validate a single ticket; if omitted, validate all
- `--warn`: also print warnings (default: only errors)

Behavior:
1. If a ticket ID is given: validate just that file, exit 1 if any errors
2. If no ID given: validate all tickets, exit 1 if any ticket has errors
3. Print results in a readable format

Human output:
```
✓ TKT-001 valid
✗ TKT-002 invalid:
    state: "blocked" is not a valid state (did you mean "in-progress"?)
    blocked_by: TKT-999 does not exist
✓ TKT-003 valid

2 tickets checked, 1 invalid.
```

JSON output:
```json
{
  "valid": ["TKT-001", "TKT-003"],
  "invalid": {
    "TKT-002": [
      {"field": "state", "message": "\"blocked\" is not a valid state"},
      {"field": "blocked_by[0]", "message": "TKT-999 does not exist"}
    ]
  }
}
```

Exit codes:
- `0`: all valid
- `1`: one or more validation errors
- `2`: internal error (file unreadable, etc.)

## Acceptance criteria
- [ ] `docket validate TKT-001` on a valid file exits 0
- [ ] `docket validate TKT-001` on a file with wrong state exits 1 with clear message
- [ ] `docket validate TKT-001` where ID in file doesn't match filename exits 1
- [ ] `docket validate TKT-001` where `blocked_by` references a non-existent ticket exits 1
- [ ] `docket validate` with no args validates all tickets and reports per-ticket results
- [ ] Cycle detection: A blocks B blocks A → exits 1 with cycle description
- [ ] `--format json` outputs the correct JSON structure
- [ ] `--warn` flag shows warnings in addition to errors
- [ ] `go test ./internal/store/local/...` and `./cmd/...` pass

## Notes for LLM
- This command is called by the pre-commit hook — it must be fast (no network calls)
- The cycle detection only needs to handle the local backend's `blocked_by` references
- Invalid YAML frontmatter should be caught and reported as a single error, not panicked
- `docket validate` is run by lefthook on staged files — the path passed may be absolute
