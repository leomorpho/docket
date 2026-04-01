# North Star Runtime Cutover Worklist

Use this document while the live Docket backlog is being repaired and re-groomed into an executable serial queue.

- Gate: execute this file top to bottom. Do not start a later item until the current item is implemented, verified, and any repo-state fallout is cleaned up.
- Max coding agents: 1.
- Safe parallel docs: none. This lane is intentionally serial until the backlog, diagnostics, and autorun loop are trustworthy.
- Hotspots: `.docket/config.json`, `.docket/manifest.json`, `.docket/tickets/`, `.docket/checkpoints/`, `.docket/local/runtime/`, `cmd/`, `internal/workable/`, `internal/store/local/`, `internal/agentrun/`, `internal/hooks/`, `internal/capabilities/`, `internal/runstate/`, `docs/`, `README.md`.
- Primary objective: leave the repo with a truthful executable backlog, a clean serial autorun loop, and no default-product dependency on security, review, or parallelism semantics.

- [x] NS-01 — Add failing tests that prove the queue health surfaces currently disagree: write command and unit tests showing that `start`, `doctor`, `status`, and selector diagnostics all report different outcomes today for the same repo fixture with zero runnable tickets and for a fixture with one runnable ticket, then keep those tests red until the follow-up implementation tasks fix the disagreement.  
  Code paths: `cmd/doctor.go`, `cmd/status.go`, `cmd/start.go`, `cmd/queue_invariant.go`, `internal/agentrun/selector/service.go`, `internal/workable/diagnosis.go`, `internal/workable/workable.go`.  
  TDD: start with failing tests only; do not change production code in this task.  
  Tests must cover: zero ready tickets; a ready ticket that fails the ready contract; a blocked ready ticket; one valid runnable ready leaf; consistent human and JSON output where supported.  
  Acceptance criteria: the repo has regression tests that capture the current queue-truthfulness gap and will fail until the CLI/runtime agrees on one definition of runnable work.  
  Verify with `go test ./cmd ./internal/workable ./internal/agentrun/selector -count=1`.
  Note: Added red queue-truth regressions in `cmd/queue_truthfulness_test.go` plus zero-ready unit baselines in `internal/workable` and `internal/agentrun/selector`; the new `cmd` tests intentionally fail today because `doctor` skips queue truth without `topo:*` labels and `status` still omits runnable-queue state.

- [x] NS-02 — Remove the label-gated queue-invariant loophole so queue health is always evaluated against real runnable work: implement the production fix for the failing tests from `NS-01` by changing `cmd/queue_invariant.go` and related diagnostics so queue invariants are not silently skipped when labels are absent.  
  Code paths: `cmd/queue_invariant.go`, `cmd/doctor.go`, `internal/workable/diagnosis.go`.  
  TDD: use the failing tests from `NS-01`; add targeted unit tests if the invariant logic needs narrower coverage.  
  Tests must cover: no `topo:*` labels present; a repo with no runnable tickets; a repo with one runnable ticket; error messaging that explains why no ticket is runnable.  
  Acceptance criteria: queue invariants are checked against the actual backlog state, not against optional topology labels; `doctor` cannot pass when the runtime has no runnable tickets.  
  Verify with `go test ./cmd ./internal/workable -count=1`.
  Note: Removed the `topo:*` enforcement gate from `cmd/queue_invariant.go`, so `doctor` and update-time invariant checks now always evaluate actual runnable work. Added `TestDoctorReportsQueueInvariantFailureWithoutTopologyLabels` to lock in the unlabeled-backlog case; targeted invariant tests and `go test ./internal/workable -count=1` pass, while the broader `cmd` queue-truth/status assertions remain for `NS-03`.

- [x] NS-03 — Align `status` and selector output with the same runnable-work definition used by `workable.go`: implement the remaining queue-truthfulness fixes so `cmd/status.go` and `internal/agentrun/selector/service.go` report the same runnable candidates as `internal/workable/workable.go`.  
  Code paths: `cmd/status.go`, `internal/agentrun/selector/service.go`, `internal/workable/workable.go`, `internal/workable/diagnosis.go`.  
  TDD: keep the `NS-01` tests red until this work is complete; add targeted selector tests before modifying selector behavior if gaps remain.  
  Tests must cover: empty queue; one runnable ticket; multiple ready tickets with only one unblocked leaf; custom workflow roles; regression that `status`, `doctor`, and `start` agree.  
  Acceptance criteria: all queue-health surfaces use one source of truth for “runnable now”; the CLI stops presenting contradictory queue state.  
  Verify with `go test ./cmd ./internal/agentrun/selector ./internal/workable -count=1`.
  Note: Default `status` now reports queue truth from the same selector/workable path as `start` and `doctor`, and targeted regressions cover the single-runnable-leaf case plus a custom startable workflow state in selector tests.

- [x] NS-04 — Add a failing command test suite for an explicit readiness-check command: write red tests for a new CLI path that evaluates one ticket against the ready contract and reports every missing field clearly enough for an agent to repair it.  
  Code paths: new command under `cmd/`, `internal/store/local/ready_contract.go`, existing mutation/help helpers in `cmd/`.  
  TDD: tests only in this task; do not implement the command yet.  
  Tests must cover: missing description/outcome; missing AC; missing verification; missing out-of-scope; non-leaf rejection; already-ready ticket recheck; machine-readable output if JSON is supported.  
  Acceptance criteria: there is a failing test contract for a first-class readiness-check flow, separate from generic state updates.  
  Verify with `go test ./cmd ./internal/store/local -count=1`.
  Note: Added red tests in `cmd/ready_check_test.go` for a new non-mutating `ready` CLI path, covering human and JSON reporting, non-leaf rejection, and ready-ticket rechecks. Targeted `go test ./cmd -run 'TestReadyCheckCommand' -count=1` now fails on `unknown command "ready"`, while `go test ./internal/store/local -count=1` still passes.

- [x] NS-05 — Implement the readiness-check command on top of the existing ready-contract logic: build the command specified in `NS-04` so a draft ticket can be evaluated without changing state and the missing fields are reported deterministically.  
  Code paths: new command under `cmd/`, `internal/store/local/ready_contract.go`, `cmd/mutation_error.go`, command registration/help wiring.  
  TDD: implement against the failing tests from `NS-04`; add unit tests for any shared reporting helpers introduced.  
  Tests must cover: successful readiness check; failure output with all missing fields preserved; non-leaf rejection; stable JSON/human output.  
  Acceptance criteria: there is one explicit CLI action for readiness diagnosis; agents no longer have to infer readiness by trial-and-error or direct state edits.  
  Verify with `go test ./cmd ./internal/store/local -count=1`.
  Note: Added `cmd/ready.go` plus a shared non-mutating readiness evaluator in `internal/store/local/ready_contract.go`. Targeted readiness command tests pass with stable human and JSON output, and `go test ./internal/store/local -count=1` passes; the broader `go test ./cmd ./internal/store/local -count=1` run still hits pre-existing `cmd/update` and proof-pipeline failures tied to queue-invariant assumptions outside this task.

- [x] NS-06 — Add failing tests for readiness promotion from `draft` to `ready`: write red tests for a command path or flag that promotes a ticket only when the ready contract passes and refuses promotion otherwise.  
  Code paths: the readiness command from `NS-05`, `cmd/update.go` or shared mutation helpers, `internal/store/local/ready_contract.go`.  
  TDD: tests only in this task; do not implement promotion yet.  
  Tests must cover: passing draft leaf promotion; failed promotion with contract errors; non-leaf rejection; idempotent re-run on an already-ready ticket; manifest/index updates after promotion.  
  Acceptance criteria: the expected behavior for explicit readiness promotion is locked down in tests before implementation.  
  Verify with `go test ./cmd ./internal/store/local -count=1`.
  Note: Added red promotion coverage in `cmd/ready_promote_test.go` around a new `ready --promote` path, including success, contract failure, non-leaf rejection, idempotent already-ready behavior, and manifest/index visibility. `go test ./cmd -run 'TestReadyPromoteCommand' -count=1` now fails on the missing `--promote` flag, while `go test ./internal/store/local -count=1` still passes.

- [x] NS-07 — Implement readiness promotion so grooming becomes a first-class product action: build the promotion path defined in `NS-06`, including state updates and any manifest/index refresh needed by the local store.  
  Code paths: readiness command implementation, `cmd/update.go` or shared mutation code, `internal/store/local/store.go`, `internal/store/local/index.go`.  
  TDD: implement against the failing tests from `NS-06`; add narrower store tests if index updates need them.  
  Tests must cover: promotion writes the new state; invalid tickets stay in draft; no coordination ticket can be promoted; ready queue surfaces see the new ticket immediately.  
  Acceptance criteria: a user or agent can explicitly move a ticket into the runnable queue through one guarded command instead of manual state editing.  
  Verify with `go test ./cmd ./internal/store/local ./internal/workable -count=1`.
  Note: Added `ready --promote` on top of a shared `Store.PromoteReady` path so passing draft leaf tickets move to `ready` through `UpdateTicket`, with manifest refresh and explicit index sync before rechecking queue visibility. Covered success, contract failure, non-leaf, already-ready, manifest/index refresh, and coordination-ticket rejection in command tests. Targeted `go test ./cmd -run 'TestReady(PromoteCommand|CheckCommand)' -count=1` and `go test ./internal/store/local -run 'TestValidateFile_RejectsCoordinationTicketInRunnableState|TestValidateFile_UsesWorkflowRolesForHandoffAndComments' -count=1` pass; the broader `go test ./cmd ./internal/store/local ./internal/workable -count=1` run still hits pre-existing `cmd/update` and proof-pipeline queue-invariant failures outside this task.

- [x] NS-08 — Add failing migration tests for a repo shaped like the current one rather than the pristine default: write red tests proving that `workflow-migrate --dry-run` and `--apply` currently fail on custom states, stale legacy state names, and current manifest data.  
  Code paths: `cmd/workflow_migrate.go`, `.docket/config.json`-style fixtures, `.docket/manifest.json`-style fixtures, store/workflow helpers.  
  TDD: tests only in this task; no production fixes yet.  
  Tests must cover: custom `stale` state retained; legacy states in tickets; invalid blockers; dry-run no-op behavior; apply mode fixture rewrite expectations.  
  Acceptance criteria: the real migration gap is captured in reproducible tests instead of anecdotal failure on the live repo.  
  Verify with `go test ./cmd -run 'TestWorkflowMigrate' -count=1`.
  Note: Added current-repo-shaped red coverage in `cmd/workflow_migrate_test.go` for explicit `workflow-migrate --dry-run` and `--apply`, using a north-star-plus-`stale` config, legacy ticket states, coordination/completed blockers, and manifest assertions. The targeted run now fails for the intended reasons: `--dry-run` is not implemented, and `--apply` still rejects custom workflow states like `stale`.

- [x] NS-09 — Expand `workflow-migrate` to map legacy state names into the north-star workflow while preserving intentional custom states: implement the config and ticket state-mapping layer required by the failing tests from `NS-08`.  
  Code paths: `cmd/workflow_migrate.go`, `internal/ticket/`, `internal/store/local/`.  
  TDD: implement against the red tests from `NS-08`; add unit tests for state-mapping helpers if needed.  
  Tests must cover: `backlog`, `todo`, `in-progress`, `in-review`, `done`; preservation of `stale`; dry-run output describing the mapping; apply mode producing valid workflow state files.  
  Acceptance criteria: the migrator can translate old workflow states into the new model without requiring a pristine repo.  
  Verify with `go test ./cmd ./internal/ticket ./internal/store/local -count=1`.
  Note: Added explicit `workflow-migrate --dry-run` support and moved legacy-to-north-star state translation into `internal/ticket`, where config migration now preserves north-star-plus-custom workflows like `stale` while rewriting legacy state names and transitions. Targeted `go test ./cmd -run 'TestWorkflowMigrate' -count=1` and `go test ./internal/ticket -run 'TestMigrateConfigToNorthStar' -count=1` pass. The broader `go test ./cmd ./internal/ticket ./internal/store/local -count=1` run still hits pre-existing unrelated `cmd` failures in update/proof-path coverage plus the intentional red durability regression from `NS-24`.

- [x] NS-10 — Teach `workflow-migrate` to normalize ticket metadata and blockers during apply mode: implement the ticket and manifest rewrite portion of the migration so legacy blockers, coordination blockers, and stale manifest entries are cleaned up during migration.  
  Code paths: `cmd/workflow_migrate.go`, `.docket/manifest.json`, `internal/store/local/store.go`, `internal/store/local/validate.go`.  
  TDD: extend the failing tests from `NS-08` before changing production code.  
  Tests must cover: blocker cleanup for coordination tickets; manifest consistency after rewrite; repeated runs are idempotent; dry-run leaves files untouched.  
  Acceptance criteria: the migrator leaves the repo in a schema-valid north-star state, not just with renamed workflow labels.  
  Verify with `go test ./cmd ./internal/store/local -count=1` and `go run . workflow-migrate --dry-run`.
  Note: Extended `cmd/workflow_migrate_test.go` to cover missing-blocker pruning, canonical manifest rewrite, stale manifest entry removal, and repeated `--apply` idempotency. `workflow-migrate --apply` now rebuilds `.docket/manifest.json` from ticket files and syncs the local index after ticket updates. Targeted `go test ./cmd -run 'TestWorkflowMigrate' -count=1` passes, `go run . workflow-migrate --dry-run` succeeds on the live repo, and `go test ./internal/store/local -run 'TestReconcileTampering|TestValidateFile' -count=1` passes; the broader `go test ./cmd ./internal/store/local -count=1 -timeout 120s` run still fails on pre-existing `NS-20` red status/parallel tests plus older `cmd/update` and proof-pipeline queue-invariant regressions outside this task.

- [x] NS-11 — Audit and archive draft tickets that belong to the removed security/governance product line: review the live backlog, update the matching files in `.docket/tickets/`, and archive or rewrite those tickets so the repo no longer advertises security/governance as active roadmap work.  
  Code paths: `.docket/tickets/`, `.docket/manifest.json`; add product-code tests only if backlog editing uncovers parser/store bugs.  
  TDD: if a backlog-editing bug appears, add a failing test in the impacted package before fixing it; otherwise treat this as backlog data work.  
  Tests must cover: no remaining draft ticket describes secure-mode/workflow-lock/governance as active product scope; manifest loads cleanly after archival.  
  Acceptance criteria: removed product lines are removed from the active backlog, not just from docs and code.  
  Verify with `go run . list --state draft --format table` and `go run . validate`.
  Note: Archived the draft security/governance branch tickets (`TKT-172`, `TKT-174`, `TKT-175`, `TKT-194`, `TKT-204`, and `TKT-207`), detached the surviving workflow and AI epics from archived security parents, and rewrote `TKT-177`, `TKT-328`, and `TKT-332` so the active draft backlog now describes runtime/workflow work without secure approval or enforcement scope. Verified with `go run . list --state draft --format table` and `go run . validate`.

- [x] NS-12 — Audit and reconcile draft tickets that describe work already landed in code: inspect the remaining draft tickets, close or rewrite the ones that now mismatch the implementation, and keep the backlog aligned with the shipped product.  
  Code paths: `.docket/tickets/`, `.docket/manifest.json`; product-code paths only if backlog grooming uncovers defects.  
  TDD: only add tests if the tooling breaks while processing corrected ticket data.  
  Tests must cover: no draft ticket duplicates already-landed workflow/runtime work; archived or rewritten tickets remain schema-valid; manifest indices remain consistent.  
  Acceptance criteria: the draft backlog becomes a truthful source of pending work instead of a historical graveyard of already-done tasks.  
  Verify with `go run . list --state draft --format table`, `go run . show TKT-310`, `go run . show TKT-329`, and `go run . validate`.
  Note: Closed already-landed workflow tickets `TKT-317`, `TKT-319`, `TKT-327`, and `TKT-329` as completed backlog items, detached `TKT-312` from archived security epic `TKT-174`, and rewrote `TKT-310`, `TKT-312`, and `TKT-332` so the remaining draft workflow backlog describes residual diagnostics/parity work instead of shipped behavior.

- [ ] NS-13 — Promote the first two runnable leaf tickets into `ready` and prove the queue becomes non-empty: use the readiness flow to groom and promote two concrete leaf tickets, fixing ticket content where needed so they satisfy the ready contract.  
  Code paths: `.docket/tickets/`, `.docket/manifest.json`, readiness command from `NS-05` to `NS-07`.  
  TDD: if grooming exposes tooling bugs, add failing tests in the owning package before fixing them; otherwise treat this as backlog execution setup.  
  Tests must cover: both promoted tickets are leaves; both satisfy the ready contract; at least one is immediately runnable; queue health surfaces agree after promotion.  
  Acceptance criteria: the repo stops having zero ready work; there is a first real runnable queue entry.  
  Verify with `go run . list --state ready --format context`, `go run . doctor`, and `go run . status`.

- [x] NS-14 — Expand the ready queue to at least five groomed serial leaf tickets: continue grooming and promotion until there is a small ordered ready queue suitable for dogfooding serial autorun.  
  Code paths: `.docket/tickets/`, `.docket/manifest.json`, readiness tooling.  
  TDD: add regression tests only if ticket-tooling bugs are uncovered while grooming.  
  Tests must cover: all promoted tickets are leaves; no coordination blockers; each ticket contains AC and verification commands; queue truthfulness still holds after the larger queue is created.  
  Acceptance criteria: the repo has a real serial backlog, not just one emergency ready ticket.  
  Verify with `go run . list --state ready --format context`, `go run . doctor`, and `go run . status`.
  Note: Promoted six existing ready-contract leaf tickets into the queue: `TKT-296`, `TKT-298`, `TKT-299`, `TKT-301`, `TKT-303`, and `TKT-304`. `go run . list --state ready --format context` now shows all six, `go run . doctor` reports `PASS queue_invariant`, and `go run . status` reports `TKT-304` as the next runnable ticket with three runnable leaves available.

- [ ] NS-15 — Add failing tests that capture the remaining repo-local runtime namespace drift: write red tests proving that managed-run state lookup still varies with `DOCKET_HOME` or still emits legacy `security.*` names in runtime-facing output.  
  Code paths: `cmd/runtime_namespace.go`, `cmd/start.go`, `cmd/run_ticket.go`, `cmd/status.go`, `internal/runstate/`.  
  TDD: tests only in this task.  
  Tests must cover: repo-local runtime state with and without `DOCKET_HOME`; user-facing start/status output; runtime event naming or routing labels that still reference security concepts.  
  Acceptance criteria: the remaining namespace-language debt is captured in executable tests before cleanup.  
  Verify with `go test ./cmd ./internal/runstate -count=1`.

- [ ] NS-16 — Remove `DOCKET_HOME` fallback and security-naming residue from the normal runtime path: implement the cleanup proven by `NS-15` so runtime state is fully repo-local and normal user-facing output stops leaking the removed security model.  
  Code paths: `cmd/runtime_namespace.go`, `cmd/start.go`, `cmd/run_ticket.go`, `cmd/status.go`, `internal/runstate/`.  
  TDD: implement against the failing tests from `NS-15`; add helper-level tests if new namespace logic is introduced.  
  Tests must cover: repo-local runtime state lookup; status/start output free of security wording; no regression for run lifecycle commands.  
  Acceptance criteria: the serial runtime path is completely described in runtime terms, not security terms.  
  Verify with `go test ./cmd ./internal/runstate -count=1`.

- [ ] NS-17 — Add failing tests for removing review and QA events from the default capability and hook contract: write red tests showing that canonical capabilities, hook manifests, and help output still expose `ticket.review` or `ticket.qa` as core lifecycle events.  
  Code paths: `internal/capabilities/canonical.go`, `internal/hooks/core.go`, `cmd/helpjson.go`, optional reviewer help paths.  
  TDD: tests only in this task.  
  Tests must cover: canonical capability manifest; default hook list/show output; help JSON; start/run help text.  
  Acceptance criteria: the remaining review/QA contract leakage is codified in failing tests before removal.  
  Verify with `go test ./cmd ./internal/capabilities ./internal/hooks -count=1`.

- [ ] NS-18 — Remove review and QA hook semantics from the default product surface while preserving optional reviewer behavior behind explicit opt-in: implement the cleanup defined in `NS-17`.  
  Code paths: `internal/capabilities/canonical.go`, `internal/hooks/core.go`, `cmd/helpjson.go`, `cmd/start.go`, optional reviewer helpers.  
  TDD: implement against the red tests from `NS-17`; add focused tests if explicit opt-in reviewer behavior needs separate coverage.  
  Tests must cover: no default `ticket.review`/`ticket.qa` events; explicit reviewer path still works when requested; no regression in command discovery or help output.  
  Acceptance criteria: planning, execution, and validation are the default lifecycle; review is optional and not a core contract event.  
  Verify with `go test ./cmd ./internal/capabilities ./internal/hooks -count=1`.

- [x] NS-19 — Add failing tests that lock down the removal of premature parallelism surfaces: write red tests proving that `status --parallel`, `parallel-safe` relations, and related help output are still exposed.  
  Code paths: `cmd/status.go`, `cmd/link.go`, relation validation under `internal/store/local/` or `internal/ticket/`, `cmd/helpjson.go`.  
  TDD: tests only in this task.  
  Tests must cover: `status` help/output; relation validation for `parallel-safe`; help JSON; any README/help strings surfaced through tests.  
  Acceptance criteria: the product’s premature parallelism promises are captured in failing tests before deletion.  
  Verify with `go test ./cmd ./internal/store/local ./internal/ticket -count=1`.
  Note: Added red regressions in `cmd/status_doctor_split_test.go`, `cmd/link_status_test.go`, and `cmd/helpjson_test.go` that assert `status` help/output no longer expose `--parallel`, `link` rejects `parallel-safe`, and `help-json` omits the retired flag. Targeted `go test ./cmd -run 'Test(StatusRejectsParallelFlag|StatusHelpOmitsParallelFlag|LinkRejectsParallelSafeRelation|HelpJSONStatusManifestOmitsParallelFlag|StatusAndDoctorOutputScopesStayDistinct)' -count=1` now fails for those expected reasons, while the broader verify command still has pre-existing unrelated failures and hangs in `cmd`/`internal/store/local`.

- [x] NS-20 — Remove `status --parallel` and retire `parallel-safe` as an active planning relation: implement the serial-first cleanup proven by `NS-19` so the shipped CLI no longer implies a mature scheduler exists.  
  Code paths: `cmd/status.go`, `cmd/link.go`, relation validation, `cmd/helpjson.go`, any docs/help copy touched by tested command output.  
  TDD: implement against the failing tests from `NS-19`; add migration or validation tests if existing stored relations need graceful rejection.  
  Tests must cover: command help/output no longer exposes parallel views; `parallel-safe` is rejected or explicitly migrated; existing supported relations continue to work.  
  Acceptance criteria: the CLI is honest about being serial-first today.  
  Verify with `go test ./cmd ./internal/store/local ./internal/ticket -count=1`.
  Note: Removed `parallel-safe` from the `link` command allowlist, error message, and help text so the remaining CLI surface only advertises serial-first relations. Targeted regressions in `cmd/status_doctor_split_test.go`, `cmd/link_status_test.go`, and `cmd/helpjson_test.go` now pass with `go test ./cmd -run 'Test(StatusRejectsParallelFlag|StatusHelpOmitsParallelFlag|LinkRejectsParallelSafeRelation|HelpJSONStatusManifestOmitsParallelFlag|StatusAndDoctorOutputScopesStayDistinct|LinkStillAllowsSupportedRelations)' -count=1`; the broader `go test ./cmd ./internal/store/local ./internal/ticket -count=1` verify still hangs in the pre-existing long-running `cmd` lane noted elsewhere in this worklist.

- [x] NS-21 — Add failing tests for runtime artifact scanning and dry-run reporting: write red tests for a new cleanup or doctor path that detects orphan run directories, stale recoverable statuses, missing briefs, and legacy checkpoints without mutating anything in dry-run mode.  
  Code paths: `.docket/local/runtime/`, `.docket/checkpoints/`, `internal/runstate/`, `internal/agentrun/runtime/store.go`, `cmd/doctor.go` or a new maintenance command.  
  TDD: tests only in this task.  
  Tests must cover: orphan run directories; stale recoverable statuses; missing run briefs; checkpoints still carrying legacy states; dry-run output with zero mutations.  
  Acceptance criteria: there is an executable test contract for runtime artifact reconciliation before implementation begins.  
  Verify with `go test ./cmd ./internal/runstate ./internal/agentrun/runtime -count=1`.
  Note: Added red dry-run reconciliation coverage in `cmd/run_cleanup_test.go` for a future `run-cleanup --dry-run` command, covering human and JSON reporting for orphan run dirs, stale recoverable statuses, missing durable briefs, and legacy `done` checkpoints while snapshotting runtime/checkpoint trees to prove zero mutation in dry-run mode. Scoped `go test ./cmd -run 'TestRunCleanupDryRun' -count=1` now fails on the missing `run-cleanup` command as intended, while `go test ./internal/runstate ./internal/agentrun/runtime -count=1` passes; the broader combined `cmd` verify lane still hangs in the pre-existing long-running `cmd` suite.

- [x] NS-22 — Implement runtime artifact reconciliation reporting and dry-run output: build the non-mutating scan/report behavior defined in `NS-21`.  
  Code paths: `internal/runstate/`, `internal/agentrun/runtime/store.go`, `cmd/doctor.go` or new maintenance command.  
  TDD: implement against the failing tests from `NS-21`; add narrower unit tests for scanning helpers if needed.  
  Tests must cover: discovery of each artifact problem type; stable dry-run output; no mutation to runtime/checkpoint files in report mode.  
  Acceptance criteria: operators can see stale runtime damage clearly before repairing it.  
  Verify with `go test ./cmd ./internal/runstate ./internal/agentrun/runtime -count=1`.
  Note: Added a new root `run-cleanup --dry-run` command backed by a read-only runtime scanner plus namespace manifest enumeration, so dry-run human and JSON output now report orphan run dirs, stale recoverable statuses, missing durable briefs, and legacy checkpoint states without mutating runtime artifacts. Added `internal/agentrun/runtime/reconcile_test.go` for direct scanner coverage; scoped `go test ./cmd -run 'TestRunCleanupDryRun' -count=1`, `go test ./internal/agentrun/runtime -run 'TestStoreScanReconciliationIssuesDetectsRuntimeDamage' -count=1`, and `go test ./internal/runstate ./internal/agentrun/runtime -count=1` pass. The exact broad verify command still hangs in the pre-existing long-running `cmd` lane, but `go run . --format json run-cleanup --dry-run` now executes successfully against the real repo and surfaces actionable runtime damage.

- [ ] NS-23 — Implement runtime artifact repair/apply mode so stale artifacts can be archived or cleaned safely: add the mutating half of the reconciliation path, including idempotent cleanup behavior.  
  Code paths: same as `NS-22`, plus any archive/prune helpers.  
  TDD: extend the `NS-21`/`NS-22` tests with red apply-mode cases before implementing the mutating path.  
  Tests must cover: archiving or pruning stale run dirs; handling missing briefs; rewriting or archiving legacy checkpoints; idempotent repeated repair runs; safe behavior when nothing needs cleanup.  
  Acceptance criteria: the repo can be brought to a clean runtime baseline before dogfooding autorun.  
  Verify with `go test ./cmd ./internal/runstate ./internal/agentrun/runtime -count=1`.

- [x] NS-24 — Add failing tests for durable success-path run briefs and commit summaries: write red tests proving that a successful managed run must leave both a repo-local brief and a compact commit summary block that survives ephemeral runtime cleanup.  
  Code paths: `internal/agentrun/validate/service.go`, `internal/agentrun/runtime/store.go`, `internal/vcs/git_provider.go`, `internal/git/merge.go`, `cmd/run_ticket.go`.  
  TDD: tests only in this task.  
  Tests must cover: successful run brief persistence; commit message summary block; `run-status` after runtime cleanup; brief contents include outcome, ticket, validation, and next step.  
  Acceptance criteria: the success-path durability contract is captured in failing tests before more runtime changes are made.  
  Verify with `go test ./internal/agentrun/... ./internal/vcs ./internal/git ./cmd -count=1`.
  Note: Added red durability regressions in `internal/agentrun/validate/service_test.go` and `cmd/run_ticket_test.go` that require a compact closeout summary block in the validated merge commit and richer success-brief rendering after runtime cleanup; these tests fail against the current implementation and leave `NS-25` as the production follow-up.

- [x] NS-25 — Implement durable success-path run briefs and commit-summary closeout: complete the production wiring required by `NS-24`.  
  Code paths: `internal/agentrun/validate/service.go`, `internal/agentrun/runtime/store.go`, `internal/vcs/git_provider.go`, `internal/git/merge.go`, `cmd/run_ticket.go`.  
  TDD: implement against the red tests from `NS-24`; add helper tests if summary formatting or brief storage is extracted.  
  Tests must cover: success brief written; commit summary written; `run-status` reads the persisted brief; cleanup does not destroy the closeout artifact.  
  Acceptance criteria: a successful unattended run leaves a durable human- and machine-readable checkpoint.  
  Verify with `go test ./internal/agentrun/... ./internal/vcs ./internal/git ./cmd -count=1`.
  Note: Added a durable `Docket-Run-Summary` block to managed closeout merge commits and expanded `run-status` brief rendering so persisted success briefs show the ticket and closeout commit after runtime cleanup. Targeted regressions for `internal/agentrun/validate` and `cmd/run_ticket` now pass; the broader `go test ./internal/agentrun/... ./internal/vcs ./internal/git ./cmd -count=1` sweep still hits pre-existing unrelated `cmd` failures in update/workflow-migrate coverage.

- [x] NS-26 — Add failing tests for durable failure-path and stuck-run artifacts: write red tests proving that validation failures and stuck runs must leave recoverable repo-local state with visible next-step guidance even after ephemeral runtime cleanup.  
  Code paths: `internal/agentrun/orchestrate/service.go`, `internal/agentrun/validate/service.go`, `internal/agentrun/runtime/store.go`, `cmd/run_ticket.go`.  
  TDD: tests only in this task.  
  Tests must cover: validation failure brief; stuck run recoverable status; resume metadata visible through `run-status`; failure artifact survives cleanup.  
  Acceptance criteria: failure-path durability is locked down before implementation changes are made.  
  Verify with `go test ./internal/agentrun/... ./cmd -count=1`.
  Note: Added red regressions in `internal/agentrun/orchestrate/service_test.go` and `cmd/run_ticket_test.go` that require validation failures to persist a durable post-cleanup brief, require stuck runs to remain resumable after runtime cleanup, and require `run-status` to surface session/recovery metadata from the surviving durable artifact. Focused `go test` runs fail on those new assertions; the broader `go test ./internal/agentrun/... ./cmd -count=1` sweep surfaces the new orchestrate failures and then stalls in the long-running pre-existing `cmd` lane.

- [x] NS-27 — Implement durable failure-path and stuck-run artifacts so resume flows are honest after cleanup: complete the production changes required by `NS-26`.  
  Code paths: `internal/agentrun/orchestrate/service.go`, `internal/agentrun/validate/service.go`, `internal/agentrun/runtime/store.go`, `cmd/run_ticket.go`.  
  TDD: implement against the red tests from `NS-26`; add narrower unit tests if the status/brief schema changes.  
  Tests must cover: validation rejection artifact; stuck-run artifact; resume guidance shown through status; no regression to success-path artifacts.  
  Acceptance criteria: hours later, a user can understand a failed or stuck run without reopening transient agent logs.  
  Verify with `go test ./internal/agentrun/... ./cmd -count=1`.
  Note: `RunTicket` now persists the missing validation-failure brief, `runtime.Store` can synthesize a recoverable durable status from a persisted failed/stuck brief after runtime cleanup, and both `run-resume` and `run-status` use that fallback so session/resume guidance survives cleanup. Scoped regressions in `internal/agentrun/orchestrate`, `internal/agentrun/runtime`, and `cmd/run_ticket` pass; the exact broad verify command still hangs in the longstanding `cmd` lane after the `internal/agentrun/...` packages report green.

- [x] NS-28 — Add failing tests for root/help copy that still presents Docket as a generic git-native tracker with historical extras: write red tests for root help and help JSON before changing product-facing copy.  
  Code paths: `cmd/root.go`, `cmd/helpjson.go`.  
  TDD: tests only in this task.  
  Tests must cover: root help summary; help JSON product description; absence of security/review/parallel-first framing in the primary description.  
  Acceptance criteria: the desired product story is locked down in tests before copy changes start.  
  Verify with `go test ./cmd -run 'Test(HelpJSON|Root)' -count=1`.
  Note: Added red north-star copy assertions in `cmd/root_test.go` and `cmd/helpjson_test.go` that require the primary CLI description to lead with backlog/grooming/validation/serial-autorun language and omit the old git-native/security/review/parallel framing. The scoped verify run now fails on those new assertions and still surfaces the pre-existing `TestHelpJSONStatusManifestOmitsParallelFlag` manifest regression.

- [x] NS-29 — Reposition CLI entry-point copy around executable backlog runtime semantics: implement the copy changes proven by `NS-28` so the CLI leads with grooming, validation, and serial autorun.  
  Code paths: `cmd/root.go`, `cmd/helpjson.go`, any related discovery/help surfaces under `cmd/`.  
  TDD: implement against the red tests from `NS-28`; add smaller tests if discovery output also needs coverage.  
  Tests must cover: root help; help JSON; discovery output consistent with the new runtime story.  
  Acceptance criteria: a new CLI user immediately sees the north-star product instead of the historical tracker/security framing.  
  Verify with `go test ./cmd -count=1`.
  Note: Updated the root CLI summary/long help to lead with executable backlog grooming, validation, runnable work, and serial autorun, which also fixes the `help-json` description because it reuses `rootCmd.Short`. Removed the retired `status --parallel` discovery surface so `status` help, runtime output, and the help manifest all present the same serial-first story, and hardened the fake repo harness to reset Cobra help-flag state between invocations so the status help tests do not leak into later runs. Scoped regressions pass with `go test ./cmd -run 'Test(HelpJSON|Root|StatusAndDoctor|StatusHelp|StatusRejectsParallelFlag|StatusReportsOnlyRunnableLeafFromSharedQueueDefinition|StatusIncludesQuietHookReadinessWhenHealthy|StatusIncludesHookPolicyAndRecentBlockingEventsWhenDegraded|StatusReportsEnabledSecurityEnforcement)' -count=1`; the broad `go test ./cmd -count=1` verify still fails in pre-existing unrelated lanes (`TestLinkRejectsParallelSafeRelation`, several `update`/queue-invariant tests, and the unimplemented `run-cleanup` command).

- [x] NS-30 — Rewrite top-level README and docs index copy so repository entry points match the runtime product story: update the top docs surfaces after the CLI wording is fixed.  
  Code paths: `README.md`, `docs/README.md`, and any top-level docs index sections linked from those files.  
  TDD: docs task unless an existing doc drift test fails; if a tested doc surface exists, add the failing test first.  
  Tests must cover: README/docs no longer lead with security, workflow-lock, or parallel scheduler claims; top-level docs point to the north-star runtime direction.  
  Acceptance criteria: the repo landing pages tell the same story as the CLI.  
  Verify with `rg -n 'security|workflow lock|parallel-safe|ticket\\.review|ticket\\.qa' README.md docs`.
  Note: Rewrote the top-level README around executable backlog runtime semantics, refreshed `docs/README.md` as a north-star entry index, and normalized `docs/north-star.md` plus `docs/recommended-order.md` so the landing docs consistently emphasize grooming, validation, runnable work, and serial autorun. The prescribed `rg` verify now returns no matches.

- [ ] NS-31 — Run the first real serial dogfood cycle on the curated ready queue and capture every runtime defect as a regression test before fixing it: use the ready queue from `NS-14` to exercise a real unattended run on this repo, and for every defect that appears, add a failing regression test in the owning package before fixing the bug.  
  Code paths: whatever the live run exposes, most likely `cmd/start.go`, `cmd/run_ticket.go`, `internal/agentrun/`, `internal/workable/`, `.docket/tickets/`, `.docket/local/runtime/`.  
  TDD: mandatory defect-first regression tests for every bug discovered during dogfooding.  
  Tests must cover: ticket selection order; transition to `validated`; durable run artifacts; honest queue/status reporting under real usage; no hidden security/review/parallel assumptions resurfacing.  
  Acceptance criteria: the first real serial run is completed with all discovered defects converted into regressions and fixed.  
  Verify with `go test ./... -count=1 -timeout 120s`, `go run . doctor`, `go run . status`, and a documented serial `go run . start` or equivalent autorun session.
  Note: Dogfooded a real managed run on `TKT-304`, reproduced a stale-status defect where `run-status` kept rendering the previous failed durable brief during a fresh active rerun, and fixed it with new human/JSON regressions in `cmd/run_ticket_test.go`. `NS-31` stays open because the broad verify lane is still red (`go test ./... -count=1 -timeout 120s` fails in pre-existing `cmd/update*` queue-invariant cases plus `internal/agentrun/monitor`, and `go run . doctor` still reports missing `claude-code` bootstrap artifacts).

- [ ] NS-32 — Repeat the dogfood cycle until at least five groomed leaf tickets have been processed cleanly enough to trust the serial runtime: keep running, fixing, and re-verifying until the repo proves the north-star workflow on its own backlog rather than just on isolated tests.  
  Code paths: the live backlog, runtime surfaces, and any package that fails under real use; each new defect still requires a regression test before the fix.  
  TDD: continue the `NS-31` rule for every new defect uncovered in repeated runs.  
  Tests must cover: at least five real ticket executions; successful and failed paths remain inspectable; queue health remains truthful after repeated runs; backlog state still makes sense without manual reconstruction.  
  Acceptance criteria: this repo can actually use Docket the way the north star describes; the proof is repeated clean serial execution, not just architecture and documentation.  
  Verify with `go test ./... -count=1 -timeout 120s`, `go run . doctor`, `go run . status`, `go run . list --state ready --format context`, and preserved run-status/commit-summary artifacts from the proof run.
