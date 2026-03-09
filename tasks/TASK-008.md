# TASK-008: `docket list` command

## Status
`[x]` done

## Depends on
TASK-006

## Command signature
```
docket list [--state <state|open>] [--label <label>] [--priority <int>] [--unblocked] [--include-archived] [--format json|table|context]
```

## Flags
- `--state`: filter by state; `open` = all except done/archived (default: `open`)
- `--label`: filter by label (repeatable: `--label bug --label feature`)
- `--priority`: max priority to show (e.g. `2` = P1 and P2 only)
- `--unblocked`: exclude tickets with non-empty `blocked_by`
- `--include-archived`: include archived tickets
- `--format`: `table` (default), `json`, `context`

## Output formats

### table
```
ID        STATE        PRI  TITLE                         LABELS
TKT-001   in-progress  P1   Add auth middleware            feature,llm-only
TKT-003   todo         P1   Fix login bug                  bug
TKT-005   backlog      P2   Auth refactor                  feature
```

### json
```json
[{"id":"TKT-001","state":"in-progress","priority":1,"title":"Add auth middleware","labels":["feature","llm-only"],"is_blocked":false}]
```

### context (LLM-optimized, minimal tokens)
```
[TKT-001] P1 in-progress | Add auth middleware | labels:feature,llm-only
[TKT-003] P1 todo        | Fix login bug       | labels:bug
[TKT-005] P2 backlog     | Auth refactor       | BLOCKED by TKT-001
```

## Behavior
- Sort: priority ascending, then `created_at` ascending within same priority
- Empty result: print "No tickets found." in table format, `[]` in json

## File to create
`cmd/list.go`

## Acceptance criteria
- [x] Default shows only open tickets (not done/archived)
- [x] `--state done` shows only done tickets
- [x] `--unblocked` excludes tickets with non-empty `blocked_by`
- [x] `--format context` outputs one compact line per ticket
- [x] `--format json` outputs valid JSON array
- [x] Sorted by priority then created_at
- [x] `go test ./cmd/...` passes
