# TASK-009: `docket show` command

## Status
`[x]` done

## Depends on
TASK-006

## Command signature
```
docket show <TKT-NNN> [--format json|md|human|context]
```

## Formats

### human (default)
...

## File to create
`cmd/show.go`

## Acceptance criteria
- [x] `docket show TKT-001` prints human-readable output
- [x] `docket show TKT-001 --format md` prints raw markdown file content
- [x] `docket show TKT-001 --format json` outputs valid JSON with all fields
- [x] `docket show TKT-001 --format context` outputs compact LLM-optimized format
- [x] `docket show TKT-999` (nonexistent) exits 1 with clear "not found" message
- [x] `go test ./cmd/...` passes
