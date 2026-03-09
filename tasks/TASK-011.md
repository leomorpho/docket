# TASK-011: `docket comment` command

## Status
`[ ]` not started

## Depends on
TASK-006

## Command signature
```
docket comment <TKT-NNN> --body <string>|-
```

## Behavior
1. Detect actor (same logic as `docket create`: `DOCKET_ACTOR` env → git config → fallback)
2. Build `ticket.Comment{At: now UTC, Author: actor, Body: body}`
3. Call `backend.AddComment(ctx, id, comment)`
4. The local backend appends to the `## Comments` section — never edits existing content

## The appended block format (local backend)
```markdown
### 2026-03-09T15:30:00Z — human:leoaudibert

Comment body here.
Multi-line is fine.

```

## Flags
- `--body` (required): comment text; `-` reads from stdin (for long comments or piped output)

## Output

### human
```
Comment added to TKT-001.
```

### json
```json
{"ticket_id": "TKT-001", "at": "2026-03-09T15:30:00Z", "author": "human:leoaudibert"}
```

## File to create
`cmd/comment.go`

## Acceptance criteria
- [ ] Appends a new `### TIMESTAMP — ACTOR` block to the `## Comments` section
- [ ] Does NOT modify any other part of the file
- [ ] `--body -` reads from stdin
- [ ] Running twice appends two separate comment blocks (not one)
- [ ] `docket comment TKT-999` exits 1 with "not found"
- [ ] `go test ./cmd/...` passes

## Notes for LLM
- This is the primary way agents record what they did — keep it simple and reliable
- The `--body -` stdin option is important: agents pipe summaries or command output as comments
- `AddComment` in the local backend must find the `## Comments` section and append after it;
  if the section doesn't exist, create it before appending
