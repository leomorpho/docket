# TASK-011: `docket comment` command

## Status
`[x]` done

## Depends on
TASK-006

## Command signature
...

## File to create
`cmd/comment.go`

## Acceptance criteria
- [x] Appends a new `### TIMESTAMP — ACTOR` block to the `## Comments` section
- [x] Does NOT modify any other part of the file
- [x] `--body -` reads from stdin
- [x] Running twice appends two separate comment blocks (not one)
- [x] `docket comment TKT-999` exits 1 with "not found"
- [x] `go test ./cmd/...` passes

## Notes for LLM
- This is the primary way agents record what they did — keep it simple and reliable
- The `--body -` stdin option is important: agents pipe summaries or command output as comments
- `AddComment` in the local backend must find the `## Comments` section and append after it;
  if the section doesn't exist, create it before appending
