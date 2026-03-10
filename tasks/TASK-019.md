# TASK-019: `docket check` — staleness and consistency

## Status
`[x]` done

## Depends on
TASK-018

## What this is
`docket check` runs a suite of pluggable health rules across all open tickets and reports
findings. Useful for periodic audit, CI, or an LLM doing a triage pass.

## Command signature
```
docket check [TKT-NNN] [--fix] [--format json|human]
```

- No args: check all open tickets
- `TKT-NNN`: check a single ticket
- `--fix`: automatically apply low-risk fixes (e.g. mark a state change suggestion)

## Rules

| ID | Severity | Description |
|----|----------|-------------|
| R001 | warn | Ticket in `in-progress` with no activity (comment/commit) in > 7 days |
| R002 | warn | Ticket has linked commits newer than last `updated_at` — state may be stale |
| R003 | warn | Ticket has open plan steps but no comments or commits in > 3 days |
| R004 | error | Ticket references files in `## Handoff` that no longer exist in the repo |
| R005 | warn | Ticket has no `## Handoff` section and state is `in-review` or `done` |
| R006 | error | `blocked_by` references a ticket that is `done` or `archived` — blocker resolved |
| R007 | warn | Ticket in `todo` or `backlog` with priority 1 and no activity in > 14 days |
| R008 | error | Schema violation (delegates to `docket validate`) |

## Output

### human
```
docket check — 8 tickets checked

  TKT-001  ⚠ R001: No activity for 9 days (state: in-progress)
  TKT-001  ⚠ R002: Commit def456 is newer than last ticket update
  TKT-003  ✗ R006: blocked_by TKT-000 which is now done — remove blocker?
  TKT-005  ✗ R004: Handoff references internal/old/file.go which no longer exists

4 findings: 2 errors, 2 warnings.
Run `docket check --fix` to apply automatic fixes.
```

### json
```json
{
  "checked": 8,
  "findings": [
    {"ticket_id": "TKT-001", "rule": "R001", "severity": "warn", "message": "No activity for 9 days", "auto_fix": false},
    {"ticket_id": "TKT-003", "rule": "R006", "severity": "error", "message": "blocked_by TKT-000 is done", "auto_fix": true}
  ],
  "summary": {"errors": 2, "warnings": 2}
}
```

## Auto-fix behavior (`--fix`)
Only applies to rules with `auto_fix: true`:
- R006: removes the resolved blocker from `blocked_by`

All other findings require human or LLM review.

## Files to create
- `cmd/check.go`
- `internal/check/rules.go` — each rule as a function `func(t *ticket.Ticket, repoRoot string) []Finding`
- `internal/check/checker.go` — runs all rules, collects findings

## Acceptance criteria
- [x] R001: ticket in-progress with `updated_at` > 7 days ago triggers warn
- [x] R006: `blocked_by` containing a done ticket triggers error
- [x] `--fix` on R006 removes the done blocker from `blocked_by` and exits 0
- [x] `--format json` outputs the JSON structure with all findings
- [x] Single ticket: `docket check TKT-001` runs all rules on just that ticket
- [x] All-clean output: "All X tickets look healthy." when no findings
- [x] `go test ./internal/check/...` passes with table-driven rule tests

## Notes for LLM
- Rules should be functions in a slice so adding new rules is one line
- Time-based rules need to accept a "now" parameter for testability
- R004 (file existence check) should use `os.Stat`, not git
