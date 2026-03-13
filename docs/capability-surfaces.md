# Capability Surfaces

Docket is one binary with two authority levels:

- Agent-safe surface: normal planning/execution commands (`list`, `show`, `create`, `comment`, `ac`, `start`, etc.) that do not require secure mode.
- Secure/admin surface: privileged commands that mutate trust or governance state and require an active secure session plus explicit confirmation.

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
