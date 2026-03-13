# Documentation Index

This directory collects reference material for the security, trust, and workflow model that Docket enforces today. The primary entry point is `security-model.md` because it covers the assumptions (who can do what), the hard limits enforced by `DOCKET_HOME`, how tampering is detected, the workflow locks that keep agents honest, and how humans recover from missteps.

- [`security-model.md`](security-model.md): Detailed walk-through of the current trust model, secure storage expectations, workflow governance, lock activation, and recovery flows. It also flags what parts of the design are intentionally deferred for future upgrades (OS keychains, biometric guards, SaaS custody).
