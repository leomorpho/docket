# Capability Surfaces

Docket is one binary with two authority levels:

- Agent-safe surface: normal planning/execution commands (`list`, `show`, `create`, `comment`, `ac`, `start`, etc.) that do not require secure mode.
- Secure/admin surface: privileged commands that mutate trust or governance state and require an active secure session plus explicit confirmation.

If no approved `workflow.lock` is active, agent-safe execution still proceeds in unsecured mode. This allows `docket start` and normal implementation work to continue without a keystore or secure session, but privileged actions and terminal transitions remain blocked.

## Privileged command set

The following commands currently enforce secure-mode checks:

- `docket secure approve`
- `docket secure set-anchor`
- `docket secure identity enroll|revoke|rotate|recover`
- `docket workflow lock generate|activate`
- `docket lock release`
- `docket worktree stop`

Each privileged command requires:

1. Active secure mode (`docket secure unlock --password ... --ttl ...`).
2. A ticket context (`--ticket TKT-NNN`).
3. Explicit confirmation (interactive prompt or `--yes`).

If secure mode is inactive, privileged commands fail with `secure mode is inactive`.

## Hook Events

Runtime contract namespace and invocation model:

- Hook namespace: `docket.hook`
- Hook invocation: `system-run`
- Hook execution: `internal-only` (discoverable, but not a general user-invoked command surface)

Docket now exposes internal lifecycle hook points with two modes:

- `advisory`: emits warnings/messages but does not block.
- `enforcement`: blocks lifecycle progression when validation fails.

Current core hook events:

- `run.start`
- `ticket.review`
- `ticket.qa`
- `ticket.privileged`

Current core enforcement hooks:

- Dedicated worktree enforcement for managed runs (`run.start`, `ticket.review`).
- Commit-linkage enforcement for managed review transitions (`ticket.review`).
- Privileged authorization enforcement (`ticket.privileged`).

## Skills

Runtime contract namespace and invocation model:

- Skill namespace: `docket.skill`
- Skill invocation: `agent-invoked`

Skills are agent-facing discovery and invocation surfaces; hooks remain system-run lifecycle controls.

Each skill definition in the runtime contract now carries first-class metadata:

- `name`: stable machine ID used across adapters and artifacts
- `title`: concise human-facing capability label
- `summary`: what the capability does
- `intent`: expected usage mode (for example `planning`, `authoring`, `review`)
- `command`: canonical CLI invocation template (may include placeholders like `{ticket_id}`)
- `triggers`: discovery hints for when an agent should consider using the capability
