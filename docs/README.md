# Documentation Index

This directory collects product and implementation reference material for Docket.

The primary entry point is now `north-star.md`, which captures the intended product direction: Docket as a ticketing + orchestration runtime for agentic development, with serial autorun, compact run artifacts, and validation as the main focus.

- [`north-star.md`](north-star.md): Product thesis, principles, intended workflow model, scheduler stance, memory stance, and explicit non-goals.
- [`recommended-order.md`](recommended-order.md): Concrete execution order for the repo so work lands in the right sequence instead of drifting across too many fronts.

The following documents remain useful as current-implementation references, but they are no longer the strategic entry point for the repo:

- [`security-model.md`](security-model.md): Detailed walk-through of the current trust model, secure storage expectations, workflow governance, lock activation, and recovery flows. It also flags what parts of the design are intentionally deferred for future upgrades (OS keychains, biometric guards, SaaS custody).
- [`capability-surfaces.md`](capability-surfaces.md): Agent-safe versus secure/admin command surfaces and the currently enforced privileged command set.
- [`worktree-flow-gap-report.md`](worktree-flow-gap-report.md): Gap analysis for the current worktree automation path versus the intended runtime behavior.
