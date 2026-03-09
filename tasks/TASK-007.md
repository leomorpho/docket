# TASK-007: `docket create` command

## Status
`[ ]` not started

## Depends on
TASK-006

## Command signature
```
docket create --title <string> [--desc <string>|-] [--priority <int>] [--labels <csv>] [--state <state>]
```

## Behavior
1. Validate: `--title` required, `--state` must be valid if given
2. Call `backend.NextID(ctx)` to get the next sequential ID
3. Detect actor: check `DOCKET_ACTOR` env var first, then `git config user.name` formatted
   as `human:<name>`, fallback `human:unknown`
4. Build `ticket.Ticket` with all fields, `CreatedAt` and `UpdatedAt` = now UTC
5. Call `backend.CreateTicket(ctx, t)`
6. Print result

## Flags
- `--title` (required)
- `--desc`: description string; use `-` to read from stdin
- `--priority`: int, default 10
- `--labels`: comma-separated, e.g. `feature,llm-only`
- `--state`: default `backlog`

## Human output
```
Created TKT-001: Add auth middleware
```

## JSON output
```json
{"id": "TKT-001", "seq": 1, "title": "Add auth middleware", "state": "backlog", "priority": 10}
```

## File to create
`cmd/create.go`

## Acceptance criteria
- [ ] Creates `.docket/tickets/TKT-001.md` with valid markdown + frontmatter
- [ ] Running again creates `TKT-002`
- [ ] `--title` missing → prints usage, exits 1
- [ ] `--state invalid` → prints error, exits 1
- [ ] `--desc -` reads description from stdin
- [ ] `DOCKET_ACTOR=agent:claude-sonnet-4-6` sets `created_by` correctly
- [ ] `--format json` outputs the new ticket as JSON
- [ ] `go test ./cmd/...` passes
