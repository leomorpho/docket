# Worktree Automation Gap Report (TKT-195)

## Scope
This report compares implemented behavior against:
- `TKT-150` (automatic agent worktree creation on start/in-progress)
- `TKT-151` (merge-back + cleanup on terminal transitions)
- `TKT-157` (resume continuity via claim/worktree/session handover)

## Current Behavior By Entry Point

### Start flow
- `cmd/start.go` calls `WorkflowManager.StartTask(...)` in `RunE`.
- `WorkflowManager.StartTask` in `internal/workflow/service.go`:
  - Attempts `GetAgentWorktreeDir` + `CreateWorktree`.
  - Falls back silently to repo root claim when worktree setup fails.
  - Returns `claimedPath`, but callers may ignore it.
- Observability gap:
  - `cmd/start.go` discards returned worktree path (`t, _, err := wf.StartTask(...)`).
  - `cmd/update.go` (`--state in-progress`) also discards returned path.
  - Agent users are not directed into the created path by default.

### MCP mutation flow
- `internal/mcp/handlers.go` `handleUpdate(...)` currently:
  - Mutates and persists ticket state before workflow side effects.
  - Then conditionally runs `wf.StartTask` / `wf.FinishTask`.
- `TKT-150` mismatch:
  - Worktree creation is attempted, but failure is silently downgraded to root-claim fallback.
  - `new_worktree_path` is only returned if non-root path survives fallback.
- `TKT-151` mismatch:
  - Done/archive path calls `wf.FinishTask` with ignored error (`_, _ = wf.FinishTask(...)`).
  - This can hide merge/cleanup/claim-release failures from the caller.
  - MCP path writes terminal state before finish workflow reports success.

### Resume flow
- `cmd/session_resume.go`:
  - Reads latest checkpoint JSON.
  - Prints metadata to stdout.
  - Does not resolve active claim/worktree.
  - Does not switch execution context to the claimed worktree.
  - Does not stream attached session logs as handover context.
- `TKT-157` mismatch:
  - Implemented flow is checkpoint-print only, not claim-based continuity handover.

## Why Automation Is Not Visible In Normal Agent Flows
1. Silent fallback behavior in `StartTask` masks worktree creation failures.
2. CLI start/update surfaces do not expose or enforce returned worktree paths.
3. MCP update path couples state persistence and workflow side effects in the wrong order and suppresses finish errors.
4. Resume command is diagnostic output, not operational handover.

## Follow-up Enforcement Targets
- `TKT-196`: require Docket-created worktrees for agent-managed runs; fail closed on fallback.
- `TKT-197`: bind run manifests to ticket/actor/worktree/workflow hash.
- `TKT-198`: enforce commit-to-ticket linkage before review/QA transitions.
- `TKT-199`: enforce Docket-native gates (worktree/review/QA/privileged transitions).

## Recommended Implementation Seams
- `internal/workflow/service.go:StartTask`:
  - Make fallback policy explicit/strict for agent flows.
  - Return typed outcome (created/fallback/failed) instead of implicit path semantics.
- `cmd/start.go` and `cmd/update.go`:
  - Surface worktree path in output and fail when strict mode requires worktree.
- `internal/mcp/handlers.go:handleUpdate`:
  - Run workflow transition atomically with state mutation, no pre-write to terminal states.
  - Propagate finish failures to MCP response.
- `cmd/session_resume.go` + store/claim integration:
  - Resolve active claim path and session artifacts for real resume handover.
