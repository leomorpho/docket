# TASK-001: Project scaffolding

## Status
`[ ]` not started

## What this is
Set up the Go project structure for `docket`. Wire the entry point, cobra CLI root, and
basic project files. No business logic — just the skeleton everything else builds on.

## Repository context
- Module: `github.com/leoaudibert/docket`
- Location: `/Users/leoaudibert/Workspace/2026/docket`
- Already exists: `go.mod`, `.gitignore`, `README.md`, `ARCHITECTURE.md`, `tasks/`
- Go version: whatever `go env GOVERSION` reports (1.21+)

## Files to create

### `main.go`
```go
package main

import "github.com/leoaudibert/docket/cmd"

func main() {
    cmd.Execute()
}
```

### `cmd/root.go`
Root cobra command:
- Binary name: `docket`
- Short: `"git-native ticket system for AI-assisted development"`
- Global flags:
  - `--format` string: output format, `human` or `json` (default: `human`)
  - `--repo` string: path to repo root (default: current working directory)
- `Execute()` function exported, called from `main.go`
- On `--repo` default: use `os.Getwd()`, propagate error clearly

### `Makefile`
```makefile
.PHONY: build install test lint

build:
	go build -o docket .

install:
	go install .

test:
	go test ./...

lint:
	go vet ./...
```

## Dependencies to add
```bash
go get github.com/spf13/cobra@latest
```

## Acceptance criteria
- [ ] `go build .` produces a `docket` binary without errors
- [ ] `./docket --help` prints usage with the correct binary name and description
- [ ] `./docket --format json --help` parses the global flag without error
- [ ] `go test ./...` passes (no tests yet, just must not fail to run)
- [ ] Directory structure: `main.go`, `cmd/root.go`, `Makefile`

## Notes for LLM
- Do not add any subcommands yet — those are TASK-007 through TASK-021
- Keep `cmd/root.go` thin: cobra wiring only, no business logic
- The `--repo` and `--format` flags must be persistent (available to all subcommands)
