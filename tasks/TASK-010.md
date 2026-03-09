# TASK-010: `docket update` command

## Status
`[x]` done

## Depends on
TASK-006

## Command signature
...

## File to create
`cmd/update.go`

## Acceptance criteria
- [x] `docket update TKT-001 --state in-progress` transitions state and updates `updated_at`
- [x] Invalid state transition (e.g. `backlog → done`) exits 1 with clear message
- [x] `--add-label` appends without duplicating an existing label
- [x] `--remove-label` on a label that doesn't exist exits 0 silently (idempotent)
- [x] `--blocked-by TKT-002` adds TKT-002 to `blocked_by` without duplicating
- [x] `--unblock TKT-002` removes TKT-002 from `blocked_by`
- [x] Unset flags do not modify their corresponding fields
- [x] `docket update TKT-999` exits 1 with "not found"
- [x] `go test ./cmd/...` passes
