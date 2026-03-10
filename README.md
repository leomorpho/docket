# docket

A git-native ticket system built for human + LLM agentic workflows.

Not JIRA. Not a markdown checklist. A structured, append-only, auditable issue tracker
that lives in your repo and works as a first-class tool for AI agents.

## What it does

- Tracks tickets as markdown files with YAML frontmatter at `.docket/tickets/TKT-XXX.md`
- Gives LLMs **forward context**: ticket descriptions, acceptance criteria, handoff summaries, plans
- Gives LLMs **backward context**: `docket blame file:line` → commit → ticket chain
- Interactive kanban board (`docket board`) with bubbletea TUI
- Pre-commit gate: `docket ac check TKT-XXX` exits 1 if acceptance criteria incomplete
- Dual surface: same binary works as CLI and as MCP server (`docket serve --mcp`)

## How it stores data

```
.docket/
├── config.json               # sequential counter, workflow config — committed
├── tickets/
│   ├── TKT-001.md            # ticket markdown source of truth — committed
│   └── TKT-001/sessions/     # conversation transcripts — gitignored by default
└── index.db                  # SQLite query cache — always gitignored, rebuilt on demand
```

Markdown ticket files are the source of truth. SQLite is a cache only and is never committed.

## Ticket states

```
backlog → todo → in-progress → [blocked*] → in-review → done → archived
```

`blocked` is computed automatically from unresolved `blocked-by` dependencies.

## Quick start

```bash
# install
go install github.com/leoaudibert/docket@latest

# initialize in your repo
docket init

# create a ticket
docket create --title "Add auth middleware" --priority 1 --labels feature

# interactive kanban board
docket board

# get LLM-optimized context for a file
docket context src/auth/middleware.go

# blame a line and get ticket history
docket blame src/auth/middleware.go:42

# run as MCP server
docket serve --mcp
```

## For LLMs

Run `docket help-json` to get a machine-readable manifest of all commands and their schemas.

Always use `docket` commands to modify ticket state — never edit `.docket/` files directly.

When starting work, run `docket list --state open --format context` to find relevant tickets.

When finishing work, run `docket session compress TKT-XXX` to generate a handoff summary.

## Design decisions

See [ARCHITECTURE.md](./ARCHITECTURE.md) for the full rationale behind every design choice.

## Development

Work is tracked in [`tasks/`](./tasks/). Each file is a self-contained atomic task with
full context so any LLM can pick it up independently.

See [`tasks/OVERVIEW.md`](./tasks/OVERVIEW.md) for the task graph and current status.
