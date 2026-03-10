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
