## Ticket system: docket

This repo uses `docket` for ticket tracking. Read these instructions carefully.

### How to read ticket data — always use the CLI

| What you want | Command |
|---|---|
| List open tickets | `docket list --state open --format context` |
| Read a specific ticket | `docket show TKT-NNN --format context` |
| Search tickets by text | `docket search "query"` |
| Find related tickets | `docket related TKT-NNN` |

**Prefer the CLI over raw markdown for reads.** The CLI applies computed fields — AC completion status, linked files, git-blame context, state history — that the raw markdown files do not include.

### At the start of every session
1. Run `docket list --state open --format context` to see open tickets.
2. Determine which ticket your work relates to.
   - If a ticket matches: note its ID (e.g. TKT-001) and run `docket show TKT-001 --format context`
   - If no ticket matches: create one with `docket create --title "..." --desc "..." --priority N`
3. Move the ticket into your repo's configured active-work state if it isn't already
   (default is `in-progress`):
   `docket update TKT-001 --state <active-state>`

### During work
- Add a comment when you make a significant decision:
  `docket comment TKT-001 --body "Chose X over Y because..."`
- When you need to find tickets by topic:
  `docket search "query"` — keyword search across ticket title, description, AC, and handoff
- When you understand why a file looks the way it does:
  `docket context <file>` — shows tickets linked to that file's history
- When you fix a line that seems odd:
  `docket blame <file>:<line>` — shows which ticket that line belongs to
- If you directly edit `.docket/tickets/TKT-001.md` in an editor:
  `docket validate TKT-001` — verify the ticket is still legal before committing

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
- Update ticket state to your configured review state (default is `in-review`):
  `docket update TKT-001 --state <review-state>`
- Write a handoff summary: `docket session compress TKT-001`
  (This will prompt you to write the summary — follow the format shown)

### Rules
- Prefer `docket show` and `docket update` for ticket work because they preserve computed context.
- Direct edits to `.docket/tickets/*.md` are allowed, but you MUST run `docket validate TKT-NNN` afterward before committing.
- Set your actor identity: `export DOCKET_ACTOR="agent:claude-sonnet-4-6"`
- For full command reference: `docket help-json`

<!-- docket:skill-pack:start -->
## Docket Skill Pack (Claude Code)

<!-- docket.skill.pack.version: docket.skills/v1 -->
<!-- docket.contract.hash: 4215e96e76b073e7c5b58adccdafa2958d65153bd2e869b3255f7560e863f2e0 -->
<!-- docket.skill.metadata.checksum: 4bbadff18330725650ed9e6233332d2f19ad7494eecfb23b9f4cb939b3b375fc -->
<!-- docket.skill.names: ticket-discovery,ticket-authoring-apply,context-optimize,learning-replay,wrap-up-readiness -->

Use `docket start` to pick up prioritized ticket work.

### Skills
- `ticket-discovery` (required)
  - title: Discover Next Ticket
  - intent: planning
  - command: docket list --state open --format context
  - triggers: session_start, resume, task_selection
  - summary: Find the next actionable ticket and inspect its working context before coding.
- `ticket-authoring-apply` (required)
  - title: Transactional Ticket Authoring
  - intent: authoring
  - command: docket ticket scaffold --format json
  - triggers: multi_line_ticket_edit, bulk_ticket_changes, automation_mode
  - summary: Use scaffold/apply commands to author or update ticket specs without fragile shell quoting.
- `context-optimize` (optional)
  - title: Compact Ticket Brief
  - intent: context
  - command: docket context-optimize {ticket_id}
  - triggers: llm_context_budget, ticket_handoff, task_brief
  - summary: Generate a bounded brief from ticket context, learnings, and recent activity.
- `learning-replay` (optional)
  - title: Replay Relevant Learnings
  - intent: quality
  - command: docket learn replay {ticket_id}
  - triggers: pre_implementation, incident_recurrence, ticket_resume
  - summary: Replay top ranked learned rules for a ticket using the same ranking model as start.
- `wrap-up-readiness` (optional)
  - title: End-of-Session Wrap-Up
  - intent: review
  - command: docket wrap-up {ticket_id}
  - triggers: session_end, pre_review, handoff
  - summary: Run wrap-up readiness checks for AC completion, handoff quality, blockers, and review transition readiness.
<!-- docket:skill-pack:end -->

