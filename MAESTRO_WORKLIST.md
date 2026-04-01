# North Star Runtime Cutover Worklist

Use this document while the live Docket backlog is being repaired and re-groomed into an executable serial queue.

- Gate: execute this file top to bottom. Do not start a later item until the current item is implemented, verified, and any repo-state fallout is cleaned up.
- Max coding agents: 1.
- Safe parallel docs: none. This lane is intentionally serial until the backlog, diagnostics, and autorun loop are trustworthy.
- Hotspots: `.docket/config.json`, `.docket/manifest.json`, `.docket/tickets/`, `.docket/checkpoints/`, `.docket/local/runtime/`, `cmd/`, `internal/workable/`, `internal/store/local/`, `internal/agentrun/`, `internal/hooks/`, `internal/capabilities/`, `internal/runstate/`, `docs/`, `README.md`.
- Primary objective: leave the repo with a truthful executable backlog, a clean serial autorun loop, and no default-product dependency on security, review, or parallelism semantics.

- [ ] NS-01 — Unify runnable-queue truthfulness across `start`, `doctor`, `status`, and selector diagnostics: write failing tests first proving that a repo with zero runnable leaves reports the same “no workable tickets” outcome everywhere, and that a repo with exactly one groomed ready leaf reports that same runnable ticket everywhere, then remove the label-gated loophole in `cmd/queue_invariant.go` and align `cmd/doctor.go`, `cmd/status.go`, `internal/agentrun/selector/service.go`, and `internal/workable/diagnosis.go` with `internal/workable/workable.go` so the runtime has one source of truth for “runnable now.”  
  Code paths: `cmd/doctor.go`, `cmd/status.go`, `cmd/queue_invariant.go`, `internal/workable/workable.go`, `internal/workable/diagnosis.go`, `internal/agentrun/selector/service.go`.  
  TDD: add failing command tests in `cmd/*_test.go` and failing unit tests in `internal/workable/*_test.go` before changing production code.  
  Tests must cover: zero ready leaves; a ready leaf that fails the ready contract; a blocked ready leaf; a single runnable ready leaf; a custom workflow config that still uses role-based startable/completed states; regression that `doctor`, `status`, and `start` never disagree about queue health.  
  Acceptance criteria: there is exactly one runtime definition of “workable”; `doctor` cannot pass the queue invariant when `start` says nothing is runnable; empty queues produce actionable reasons instead of contradictory health output.  
  Verify with `go test ./cmd ./internal/workable ./internal/agentrun/selector -count=1`.

- [ ] NS-02 — Add a first-class draft-to-ready grooming flow instead of relying on manual state fiddling: write failing command tests for a dedicated readiness command that evaluates a ticket against the runnable contract, reports every missing field clearly, and can promote a passing draft leaf to `ready`, then implement it on top of the existing ready-contract logic so grooming is an explicit product action rather than an undocumented `update --state ready` convention.  
  Code paths: create a new command under `cmd/` (for example `cmd/ready_check.go`), reuse `internal/store/local/ready_contract.go`, and wire any shared messaging into `cmd/mutation_error.go` or the existing ticket mutation helpers.  
  TDD: begin with failing command tests that exercise non-interactive CLI output and any needed store/unit tests around ready-contract reporting.  
  Tests must cover: missing summary/outcome; missing acceptance criteria; missing verification commands; missing explicit out-of-scope; a non-leaf or coordination ticket rejected from promotion; a passing draft leaf promoted cleanly to `ready`; idempotent re-check of an already ready ticket; JSON output if the surrounding command family supports JSON.  
  Acceptance criteria: a user can run one explicit command to know why a draft ticket is not runnable; passing tickets can be promoted without hand-editing state files; error output is specific enough for an agent to repair the ticket.  
  Verify with `go test ./cmd ./internal/store/local -count=1`.

- [ ] NS-03 — Make `workflow-migrate` work on the real repository instead of only the pristine shipped default: write failing tests proving that the current repo configuration, including custom states such as `stale`, legacy blockers, and old manifest/workflow metadata, can be dry-run migrated and then applied without manual surgery, then expand `cmd/workflow_migrate.go` so it can map legacy state names into the north-star model, rewrite or archive unsupported states, and normalize ticket/manifest data safely.  
  Code paths: `cmd/workflow_migrate.go`, `.docket/config.json`, `.docket/manifest.json`, `internal/store/local/store.go`, and any shared workflow helpers under `internal/ticket/` or `internal/store/local/`.  
  TDD: add failing command tests for `workflow-migrate --dry-run` and `workflow-migrate --apply` against a fixture repo shaped like the current one before broadening the migrator.  
  Tests must cover: custom states present in config; legacy tickets in `backlog`, `todo`, `in-progress`, `in-review`, or `done`; invalid blockers pointing at coordination tickets; dry-run output that describes the rewrite without mutating files; apply mode that rewrites config and tickets consistently; repeated runs that are safe and idempotent.  
  Acceptance criteria: the migrator can process a real non-pristine repo; dry-run and apply both work; the result is a clean `draft -> ready -> running -> validated -> archived` model plus any intentionally preserved non-runtime state such as `stale`.  
  Verify with `go test ./cmd ./internal/store/local -count=1` and `go run . workflow-migrate --dry-run`.

- [ ] NS-04 — Reset the live backlog by archiving or rewriting draft tickets that belong to the removed security/governance branch or that describe work already landed in code: inspect the remaining draft tickets and update `.docket/tickets/` plus `.docket/manifest.json` so the repo backlog reflects the current product instead of historical directions, adding regression tests only if tooling bugs are uncovered while editing real ticket data.  
  Code paths: `.docket/tickets/`, `.docket/manifest.json`, and any ticket-loading code that fails while processing corrected backlog data.  
  TDD: if backlog cleanup exposes parser, store, or manifest bugs, add a failing test in the impacted package before fixing the bug; otherwise treat this as a repository-data grooming task, not a feature task.  
  Tests must cover: no remaining draft ticket represents secure-mode/governance as active product work; no remaining draft ticket duplicates work already completed in code; parent tickets summarize work instead of blocking execution; manifest indices remain consistent after archival and rewrites.  
  Acceptance criteria: obsolete draft work is either archived or rewritten to match the actual product direction; the backlog stops advertising removed product lines; loading/listing tickets succeeds after the cleanup.  
  Verify with `go run . list --state draft --format table`, `go run . show TKT-172`, and `go test ./... -count=1 -run 'Test(Ticket|Manifest|List|Show)'`.

- [ ] NS-05 — Curate the first real `ready` queue for this repo by promoting a small ordered set of leaf tickets that are actually runnable under the north-star contract: use the readiness flow from the prior task to groom and promote at least five concrete leaf tickets, update their acceptance criteria and verification commands where needed, and ensure they are ordered so serial autorun can consume them without hidden prerequisites.  
  Code paths: `.docket/tickets/`, `.docket/manifest.json`, and the readiness command from `NS-02`; only change product code if grooming uncovers a defect in ticket parsing or validation.  
  TDD: if tooling defects appear while grooming live tickets, add a failing test in the impacted command/store package before fixing the defect; otherwise treat this as backlog execution setup, not feature work.  
  Tests must cover: every promoted ticket is a leaf; every promoted ticket satisfies the ready contract; there are no coordination blockers in the promoted chain; the queue has at least one immediately runnable ticket after promotion; `start`, `doctor`, and `status` all agree on the resulting queue.  
  Acceptance criteria: the repo no longer has zero ready work; there is a real serial queue of groomed leaf tickets; every promoted ticket has concrete AC and verification that an agent can execute without guessing.  
  Verify with `go run . list --state ready --format context`, `go run . doctor`, and `go run . status`.

- [ ] NS-06 — Remove the remaining runtime namespace and security-naming residue from the normal execution path: write failing tests proving that managed-run state resolution no longer depends on `DOCKET_HOME`, that runtime artifacts are stored under the repo runtime namespace, and that user-facing run/status/start output no longer uses `security.*` terminology, then simplify `cmd/runtime_namespace.go`, `cmd/start.go`, `cmd/run_ticket.go`, `cmd/status.go`, and any shared runstate helpers to match the new product language.  
  Code paths: `cmd/runtime_namespace.go`, `cmd/start.go`, `cmd/run_ticket.go`, `cmd/status.go`, `internal/runstate/`, and any helper packages still emitting legacy operation names.  
  TDD: start with failing command tests and focused unit tests for namespace resolution before deleting fallback behavior.  
  Tests must cover: repo-local runtime state with and without `DOCKET_HOME` set; status/start output free of “Security enforcement” phrasing; runtime artifacts written to the repo-local namespace; no regression for managed-run lifecycle commands that depend on runtime state lookup.  
  Acceptance criteria: the normal runtime path is fully repo-local; user-facing execution surfaces stop leaking security product language; backward-compatibility shims are deleted unless they are strictly required for reading existing artifacts.  
  Verify with `go test ./cmd ./internal/runstate -count=1`.

- [ ] NS-07 — Remove review and QA hook semantics from the default runtime contract so optional reviewer passes stop shaping the product surface: write failing tests showing that canonical capabilities, default hooks, and help output no longer advertise `ticket.review` or `ticket.qa` as core lifecycle events, then simplify the default hook/capability contract while keeping any optional reviewer implementation behind explicit opt-in paths only.  
  Code paths: `internal/capabilities/canonical.go`, `internal/hooks/core.go`, `cmd/helpjson.go`, `cmd/start.go`, and any optional reviewer helpers that still leak review semantics into default output.  
  TDD: add failing unit tests for capability and hook manifests and failing command tests for help output before removing the legacy events.  
  Tests must cover: canonical capabilities output; hook list/show output; help JSON output; start/run help text; optional reviewer pass still functions when explicitly requested, without reintroducing review as a required lifecycle concept.  
  Acceptance criteria: the default runtime contract talks about planning, execution, validation, and optional review only by exception; no core capability or default hook event requires review semantics.  
  Verify with `go test ./cmd ./internal/capabilities ./internal/hooks -count=1`.

- [ ] NS-08 — Delete premature parallelism surfaces from the product until the serial runtime is proven: write failing tests showing that user-facing commands can no longer advertise `status --parallel` or accept `parallel-safe` as a first-class planning relation, then remove or explicitly retire those surfaces in `cmd/status.go`, `cmd/link.go`, any relation validation helpers, and the relevant help/docs so the product stops promising a scheduler it does not yet have.  
  Code paths: `cmd/status.go`, `cmd/link.go`, ticket relation validation under `internal/store/local/` or `internal/ticket/`, `cmd/helpjson.go`, and any docs/help strings that still teach parallel planning.  
  TDD: begin with failing command tests for help/output/validation, then delete or hard-reject the parallel surfaces.  
  Tests must cover: `status` help/output no longer exposes parallel matrix views; linking with `parallel-safe` is rejected or migrated; existing non-parallel relations continue to work; help JSON and README/docs no longer describe safe-parallel planning as active product behavior.  
  Acceptance criteria: the shipped product surface is honest about being serial-first; there is no default CLI path that implies a mature parallel scheduler exists today.  
  Verify with `go test ./cmd ./internal/store/local ./internal/ticket -count=1`.

- [ ] NS-09 — Add runtime artifact reconciliation and legacy checkpoint cleanup so stale runs stop polluting the repo and resume surfaces: write failing tests for a reconciliation path that detects orphan run directories, stale recoverable statuses, missing briefs, and checkpoints still carrying old workflow state names, then implement a repo-local cleanup/repair command or doctor sub-check that can report and, when asked, repair or archive those artifacts safely.  
  Code paths: `.docket/local/runtime/`, `.docket/checkpoints/`, `internal/runstate/`, `internal/agentrun/runtime/store.go`, `cmd/doctor.go`, and a new or expanded runtime-maintenance command under `cmd/`.  
  TDD: add failing unit tests around artifact scanning/repair logic and failing command tests for dry-run versus apply behavior before implementing cleanup.  
  Tests must cover: orphan agent-run directories; recoverable runs with no live process; missing run briefs; checkpoints using legacy state names; dry-run output with no mutation; apply mode that archives, repairs, or prunes safely; idempotent repeated execution.  
  Acceptance criteria: stale runtime artifacts are visible and repairable; resume/status surfaces are not polluted by dead runs from old refactors; the repo can be brought to a clean runtime baseline before dogfooding autorun.  
  Verify with `go test ./cmd ./internal/runstate ./internal/agentrun/runtime -count=1`.

- [ ] NS-10 — Prove run-brief durability and commit-summary closeout end to end under both success and failure paths: write failing tests showing that a successful managed run leaves a durable repo-local brief plus a commit summary, and that stuck or validation-failed runs leave enough persisted state for `run-status` and resume flows to explain what happened without the ephemeral runtime directory, then complete any missing wiring in the run-validation/orchestration and VCS layers.  
  Code paths: `internal/agentrun/validate/service.go`, `internal/agentrun/orchestrate/service.go`, `internal/agentrun/runtime/store.go`, `internal/vcs/git_provider.go`, `internal/git/merge.go`, `cmd/run_ticket.go`.  
  TDD: begin with failing unit/integration tests in the agentrun and VCS packages before touching production code.  
  Tests must cover: successful run writes brief and commit summary; validation rejection writes a failure brief with actionable next step; stuck run writes recoverable status and visible resume metadata; `run-status` can surface the brief after cleanup; commit body contains the compact run summary block.  
  Acceptance criteria: serial autorun leaves durable human- and machine-readable artifacts on every closeout path; checking back hours later does not require reading transient agent logs to understand what happened.  
  Verify with `go test ./internal/agentrun/... ./internal/vcs ./internal/git ./cmd -count=1`.

- [ ] NS-11 — Reposition the primary entry points so the repo advertises “executable backlog runtime” instead of “git-native ticket tracker with extras”: write failing command tests for root help and help JSON, then update `cmd/root.go`, `cmd/helpjson.go`, `README.md`, `docs/README.md`, and any short product copy so the first story is grooming, validation, and serial autorun, not historical security/review/parallel features.  
  Code paths: `cmd/root.go`, `cmd/helpjson.go`, `README.md`, `docs/README.md`, and any other top-level discoverability docs surfaced by install/help flows.  
  TDD: add failing command tests for root/help output before changing CLI copy; docs do not require tests unless a doc drift check already exists.  
  Tests must cover: root help summary; help JSON product description; no primary help copy leading with secure-mode, review, or parallel planning; command discovery output consistent with the new runtime story.  
  Acceptance criteria: a new user landing on the CLI or README sees the north-star product immediately; legacy concepts only appear as explicit non-core or historical context if they appear at all.  
  Verify with `go test ./cmd -count=1` and `rg -n 'security|workflow lock|parallel-safe|ticket\\.review|ticket\\.qa' README.md docs cmd/helpjson.go cmd/root.go`.

- [ ] NS-12 — Dogfood the serial autorun loop on this repository with the newly curated ready queue and close the defects it exposes: run the repo through a real single-lane autorun cycle, require test-first fixes for every runtime defect uncovered, and do not stop until at least five groomed leaf tickets have been processed cleanly enough that the resulting status, briefs, commit summaries, and backlog state make sense without manual reconstruction.  
  Code paths: whatever the live run exposes, most likely `cmd/start.go`, `cmd/run_ticket.go`, `internal/agentrun/`, `internal/workable/`, `.docket/tickets/`, and `.docket/local/runtime/`; add regression tests in the precise package that owns each defect before fixing it.  
  TDD: every defect uncovered during dogfooding must get a failing regression test before the fix, even if the defect appears in an integration path first.  
  Tests must cover: serial selection consumes the intended ready tickets in order; successful runs land in `validated`; failed or stuck runs leave durable artifacts and honest status; backlog state after the run still satisfies queue truthfulness; no hidden security/review/parallel assumptions reappear under real usage.  
  Acceptance criteria: this repo can actually use Docket the way the north star describes; there is a real proof run, not just architecture and docs; the runtime can be left alone for hours and still be inspectable when revisited.  
  Verify with `go test ./... -count=1 -timeout 120s`, `go run . doctor`, `go run . status`, `go run . list --state ready --format context`, and a documented serial `docket start` or `docket autorun` dogfood run.
