# TASK-023: Bootstrap docket to track itself

## Status
`[ ]` not started

## Depends on
TASK-022 (everything implemented and working)

## What this is
Use the completed `docket` binary to initialize ticket tracking in the docket repo itself.
Create tickets for all remaining backlog items, close the loop.

## Why
Dogfooding. If docket is worth using, we use it for docket's own development. This also
serves as the integration test that all commands work together end-to-end in a real repo.

## Steps

1. Run `docket init` in the docket repo root
2. Copy `templates/lefthook.yml` entries into the repo's `lefthook.yml`
3. Copy `templates/CLAUDE.md` content into the repo's `CLAUDE.md` (create if missing)
4. Create a ticket for each task in `tasks/` that is not yet done, using the task title
   and linking the task file path in the description
5. Mark TASK-001 through TASK-022 tickets as `done` with a comment explaining they were
   completed before bootstrapping
6. Verify `docket board` shows a clean kanban with only remaining work
7. Run `docket check` and resolve any findings
8. Commit: `chore: bootstrap docket ticket tracking`

## Ticket creation script (run manually or as a shell one-liner)
```bash
for i in $(seq -w 1 22); do
  docket create \
    --title "$(head -3 tasks/TASK-0${i}.md | grep '^#' | sed 's/# TASK-0[0-9]*: //')" \
    --desc "See tasks/TASK-0${i}.md for full specification." \
    --state done \
    --priority $i \
    --labels chore
done
```

## Acceptance criteria
- [ ] `.docket/config.json` exists in the docket repo
- [ ] `docket list --state open` shows no stale or incorrect tickets
- [ ] `docket board` renders without errors
- [ ] `docket check` shows no errors (warnings acceptable)
- [ ] `docket validate` passes on all ticket files
- [ ] lefthook pre-commit hooks are active and tested (make a test commit touching a ticket)
- [ ] `CLAUDE.md` exists in repo root with docket instructions

## Notes for LLM
- This is a real repo operation, not a test — use the actual binary, not mocks
- The goal is a clean starting state for ongoing docket development tracked by docket itself
- After this task, all new work on docket should start with `docket list --state open`
