# docket

Executable backlog runtime for grooming, validation, runnable work, and serial autorun.

Docket keeps a repository backlog in a state that an agent can actually execute. It helps teams turn groomed leaf tickets into validated code changes, run the next executable ticket serially, and leave compact artifacts that make unattended work easy to inspect, resume, and trust.

## What Docket Optimizes For

- groomed leaf tickets that are genuinely runnable
- a hard ready contract before work starts
- truthful queue health and ticket selection
- serial autorun before broader scheduling
- validated completion instead of review-first waiting states
- compact run artifacts instead of transcript spelunking

## Default Runtime Loop

1. Groom a leaf ticket until it is bounded, testable, and ready to run.
2. Check the ready contract with `docket ready TKT-XXX`.
3. Inspect queue health with `docket doctor` and `docket status`.
4. Run the next executable work with `docket start` or `docket run-ticket TKT-XXX`.
5. Inspect the durable result with `docket run-status TKT-XXX`, ticket handoff, and commit artifacts.
6. Resume interrupted work with `docket run-resume TKT-XXX` when needed.

## Default Workflow

```text
draft -> ready -> running -> validated -> archived
```

Computed overlays such as `blocked`, `stalled`, and `needs-input` help explain runtime state without replacing the main execution path.

## Quick Start

```bash
# install
go install github.com/leomorpho/docket@latest

# initialize in your repo
docket init

# create and groom a ticket
docket create --title "Add auth middleware" --priority 1 --labels feature
docket ac add TKT-001 --desc "Middleware rejects invalid tokens" --run "go test ./..."
docket ready TKT-001

# inspect queue health and run the next executable ticket
docket doctor
docket status
docket start

# inspect or resume a managed run later
docket run-status TKT-001
docket run-resume TKT-001
```

## Core Commands

- `docket ready TKT-XXX`: evaluate one ticket against the ready contract without changing state
- `docket doctor`: check repository, runtime, and queue health
- `docket status`: show runtime ticket state and runnable queue status
- `docket start`: pick up the next executable ticket from the groomed queue
- `docket run-ticket TKT-XXX`: run one specific ticket through the managed flow
- `docket run-status TKT-XXX`: inspect durable status and resume guidance for a run
- `docket run-resume TKT-XXX`: resume a stuck or interrupted managed run
- `docket help-json`: print a machine-readable manifest of CLI commands and guidance

## Repository Layout

```text
.docket/
├── config.json               # workflow and runtime config committed with the repo
├── tickets/
│   ├── TKT-001.md            # markdown ticket source of truth
│   └── TKT-001/sessions/     # attached session logs, gitignored by default
├── checkpoints/              # durable ticket and runtime checkpoints
├── local/runtime/            # local managed-run status and artifacts
└── semantic/                 # optional local semantic index and metadata
```

Markdown ticket files remain the source of truth. Local runtime state, caches, and semantic artifacts are rebuildable support surfaces rather than committed history.

## Other Surfaces

The runtime loop is the product center of gravity. Docket also ships supporting surfaces for discovery, context, and local operator workflows:

- `docket board` for interactive ticket browsing
- `docket context` and `docket blame` for code-to-ticket context
- `docket serve --mcp` for MCP integrations
- `docket semantic ...` for local semantic search and related-ticket lookup

## Docs

- [docs/north-star.md](./docs/north-star.md): product thesis, operating model, and non-goals for the executable backlog runtime
- [docs/recommended-order.md](./docs/recommended-order.md): ordered cutover plan for landing the runtime changes in the repo
- [docs/worktree-flow-gap-report.md](./docs/worktree-flow-gap-report.md): current gap analysis for worktree automation versus the intended runtime behavior
- [ARCHITECTURE.md](./ARCHITECTURE.md): implementation rationale and subsystem design notes

## For Agents

Run `docket help-json` for a machine-readable manifest of commands, examples, and workflow guidance.

Prefer `docket show`, `docket ready`, `docket status`, and the scaffold/apply flows over ad hoc file parsing when you need computed ticket context.

Direct edits to `.docket/tickets/*.md` are allowed, but run `docket validate TKT-XXX` or `docket validate` before committing so schema and write-hash integrity are restored.

## Development

Work is tracked in [`tasks/`](./tasks/) and the current north-star cutover is sequenced in [`MAESTRO_WORKLIST.md`](./MAESTRO_WORKLIST.md).
