# TASK-006: `docket init` command

## Status
`[ ]` not started

## Depends on
TASK-005

## What this is
Initialize a `.docket/` directory in a git repo. Creates `config.json`, `tickets/` dir,
and adds gitignore entries. Safe to run multiple times (idempotent).

## Command signature
```
docket init [--repo <path>]
```

## Behavior
1. Check if `.docket/config.json` already exists → print "already initialized", exit 0
2. Create `.docket/` and `.docket/tickets/` directories
3. Write `.docket/config.json` with `ticket.DefaultConfig()`
4. Append to repo's `.gitignore` (create if missing), adding only missing lines:
   ```
   # docket
   .docket/index.db
   .docket/tickets/*/sessions/
   ```
   Check for `.docket/index.db` as the sentinel — don't append if already present.
5. Print success + next steps

## Human output
```
Initialized docket in .docket/

Next steps:
  docket create --title "My first ticket"
  docket board
```

## JSON output
```json
{"status": "ok", "path": ".docket/"}
```

## File to create
`cmd/init.go`

## Acceptance criteria
- [ ] Running in empty dir creates `.docket/config.json` with counter=0
- [ ] Running twice prints "already initialized", exits 0, does not overwrite config
- [ ] `.gitignore` gets the three docket lines appended
- [ ] Running twice does not duplicate `.gitignore` lines
- [ ] Works on a dir with no existing `.gitignore` (creates it)
- [ ] `--format json` outputs valid JSON
- [ ] `go test ./cmd/...` passes using `t.TempDir()`
