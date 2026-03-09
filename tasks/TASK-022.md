# TASK-022: lefthook config + CLAUDE.md template

## Status
`[ ]` not started

## Depends on
TASK-021

## What this is
Provide the lefthook configuration and CLAUDE.md template that any repo should add
when using docket. These are the "install docket in your repo" artifacts.

## Files to create

### `templates/lefthook.yml`
The pre-commit hooks that enforce docket discipline at commit time.

```yaml
pre-commit:
  commands:
    docket-validate:
      glob: ".docket/tickets/*.md"
      run: docket validate {staged_files}
      fail_text: "Ticket schema validation failed. Fix errors before committing."

    docket-ac-check:
      run: |
        TICKET=$(git log -1 --format="%B" 2>/dev/null | grep -oP '(?<=Ticket: )TKT-\d+' || true)
        if [ -n "$TICKET" ]; then
          docket ac check "$TICKET"
        fi
      fail_text: "Acceptance criteria for linked ticket are not all complete."
```

### `templates/CLAUDE.md`
Instructions for Claude Code (or any LLM agent) when working in a repo with docket installed.

```markdown
## Ticket system: docket

This repo uses `docket` for ticket tracking. Read these instructions carefully.

### At the start of every session
1. Run `docket list --state open --format context` to see open tickets.
2. Determine which ticket your work relates to.
   - If a ticket matches: note its ID (e.g. TKT-001) and run `docket show TKT-001 --format context`
   - If no ticket matches: create one with `docket create --title "..." --desc "..." --priority N`
3. Move the ticket to `in-progress` if it isn't already:
   `docket update TKT-001 --state in-progress`

### During work
- Add a comment when you make a significant decision:
  `docket comment TKT-001 --body "Chose X over Y because..."`
- When you understand why a file looks the way it does:
  `docket context <file>` — shows tickets linked to that file's history
- When you fix a line that seems odd:
  `docket blame <file>:<line>` — shows which ticket that line belongs to

### Before committing
- Check that acceptance criteria are met:
  `docket ac check TKT-001`
- Mark completed AC items with evidence:
  `docket ac complete TKT-001 --step N --evidence "..."`
- Include the ticket in your commit message:
  ```
  feat: add JWT middleware validation

  Ticket: TKT-001
  ```

### When finishing work
- Update ticket state: `docket update TKT-001 --state in-review`
- Write a handoff summary: `docket session compress TKT-001`
  (This will prompt you to write the summary — follow the format shown)

### Rules
- NEVER edit `.docket/tickets/*.md` files directly without running `docket validate` afterward.
- ALWAYS use `docket` commands to update ticket state — direct edits are caught by pre-commit hooks.
- Set your actor identity: `export DOCKET_ACTOR="agent:claude-sonnet-4-6"`
- For full command reference: `docket help-json`
```

### `templates/gitignore-additions`
Lines to add to a repo's `.gitignore` (also done automatically by `docket init`):
```
# docket
.docket/index.db
.docket/tickets/*/sessions/
```

## Acceptance criteria
- [ ] `templates/lefthook.yml` exists and is valid YAML
- [ ] The `docket-validate` hook runs `docket validate` on staged ticket files only
- [ ] The `docket-ac-check` hook exits 0 when commit message has no `Ticket:` trailer
- [ ] The `docket-ac-check` hook exits 1 when linked ticket has incomplete AC
- [ ] `templates/CLAUDE.md` exists with all required sections
- [ ] The CLAUDE.md covers: session start, during work, before commit, finishing
- [ ] `templates/gitignore-additions` matches what `docket init` adds

## Notes for LLM
- The lefthook `glob` on `docket-validate` means it only runs when ticket files are staged —
  commits that don't touch `.docket/` won't trigger the validation
- The `docket-ac-check` hook must be safe to run on commits with no `Ticket:` trailer (common case)
- The CLAUDE.md template should be concise — it gets read by the LLM on every session
