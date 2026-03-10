# TASK-018: Acceptance criteria commands

## Status
`[x]` done

## Depends on
TASK-017

## What this is
Commands to manage acceptance criteria on a ticket and use them as a pre-commit gate.
`docket ac check` is the core: exits 1 if any AC is incomplete, used in git hooks.

## Commands

### `docket ac add <TKT-NNN> --desc <string>`
Add a new AC item to a ticket.
Appends `- [ ] <description>` to the `## Acceptance Criteria` section.

### `docket ac complete <TKT-NNN> --step <N|description> --evidence <string>`
Mark an AC item as done.
- `--step`: either the 1-based index or a substring match of the description
- `--evidence`: what proves this criterion was met (required)

Updates the line from:
```
- [ ] Middleware validates tokens on all /api/* routes
```
to:
```
- [x] Middleware validates tokens on all /api/* routes — evidence: go test ./... passed 2026-03-09
```

### `docket ac check <TKT-NNN>`
Exit 0 if all AC items are done, exit 1 if any remain incomplete.

Used as a pre-commit gate via lefthook. Output:

```
✓ TKT-001: all 3 acceptance criteria met.
```

```
✗ TKT-001: 2 of 3 acceptance criteria incomplete:
  [ ] Invalid token returns 401 with structured error body
  [ ] Integration tests pass in CI
```

### `docket ac list <TKT-NNN>`
Print all AC items with their status.

## Output

### json (for `ac check`)
```json
{
  "ticket_id": "TKT-001",
  "complete": false,
  "total": 3,
  "done": 1,
  "remaining": [
    "Invalid token returns 401 with structured error body",
    "Integration tests pass in CI"
  ]
}
```

## Files to create
- `cmd/ac.go` (cobra parent)
- `cmd/ac_add.go`
- `cmd/ac_complete.go`
- `cmd/ac_check.go`
- `cmd/ac_list.go`

## Acceptance criteria (meta — for this task itself)
- [x] `docket ac add TKT-001 --desc "Tests pass"` appends `- [ ] Tests pass` to AC section
- [x] `docket ac complete TKT-001 --step 1 --evidence "go test passed"` marks step 1 done
- [x] `docket ac complete TKT-001 --step "Tests"` matches by substring
- [x] `docket ac check TKT-001` exits 0 when all AC done, exits 1 when any remain
- [x] `docket ac check TKT-001` with no AC items exits 0 (vacuously complete)
- [x] `--format json` on `ac check` outputs the JSON structure above
- [x] `go test ./cmd/...` passes

## Notes for LLM
- `ac complete` must update the markdown file in place — only the matching `- [ ]` line changes
- Substring match for `--step` should be case-insensitive and match the first item if ambiguous
- The exit code of `docket ac check` is the critical part — it must be reliable for hook use
