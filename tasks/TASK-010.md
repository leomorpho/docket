# TASK-010: `docket update` command

## Status
`[ ]` not started

## Depends on
TASK-006

## Command signature
```
docket update <TKT-NNN> [--state <state>] [--priority <int>] [--title <string>]
                        [--labels <csv>] [--add-label <label>] [--remove-label <label>]
                        [--blocked-by <TKT-NNN>] [--unblock <TKT-NNN>]
                        [--desc <string>|-]
```

## Behavior
1. Load existing ticket via `backend.GetTicket`
2. Apply only the flags that were explicitly set (don't zero out unset fields)
3. For `--state`: validate transition via `ticket.ValidateTransition(current, new)`
4. Set `updated_at` = now UTC
5. Call `backend.UpdateTicket(ctx, t)`
6. Print updated ticket summary

## Flags
- `--state`: new state (must be a valid transition from current state)
- `--priority`: new priority integer
- `--title`: new title
- `--labels`: replace all labels with this csv
- `--add-label`: append one label (repeatable)
- `--remove-label`: remove one label (repeatable)
- `--blocked-by`: add a blocker ticket ID
- `--unblock`: remove a blocker ticket ID
- `--desc`: replace description; `-` reads from stdin

## Output

### human
```
Updated TKT-001: state backlog → in-progress
```

### json
```json
{"id": "TKT-001", "updated_fields": ["state", "priority"], "state": "in-progress", "priority": 2}
```

## File to create
`cmd/update.go`

## Acceptance criteria
- [ ] `docket update TKT-001 --state in-progress` transitions state and updates `updated_at`
- [ ] Invalid state transition (e.g. `backlog → done`) exits 1 with clear message
- [ ] `--add-label` appends without duplicating an existing label
- [ ] `--remove-label` on a label that doesn't exist exits 0 silently (idempotent)
- [ ] `--blocked-by TKT-002` adds TKT-002 to `blocked_by` without duplicating
- [ ] `--unblock TKT-002` removes TKT-002 from `blocked_by`
- [ ] Unset flags do not modify their corresponding fields
- [ ] `docket update TKT-999` exits 1 with "not found"
- [ ] `go test ./cmd/...` passes
