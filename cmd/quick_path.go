package cmd

type llmQuickPath struct {
	Preference     string `json:"preference"`
	TicketApply    string `json:"ticket_apply"`
	BacklogApply   string `json:"backlog_apply"`
	ProofAttach    string `json:"proof_attach"`
	ProofVerify    string `json:"proof_verify"`
	AutomationHint string `json:"automation_hint"`
}

func buildLLMQuickPath() llmQuickPath {
	return llmQuickPath{
		Preference:     "Prefer transactional scaffold/apply commands over multi-step manual create/update flows.",
		TicketApply:    "docket ticket scaffold > ticket-spec.json && docket --automation ticket apply --spec ticket-spec.json",
		BacklogApply:   "docket backlog scaffold > backlog-spec.json && docket --automation backlog apply --spec backlog-spec.json",
		ProofAttach:    "docket proof add TKT-NNN --file artifacts/screenshot.png --proof-title \"Before fix\" --note \"What this screenshot proves\" --captured-at 2026-03-16T18:40:00Z --format json",
		ProofVerify:    "docket proof list TKT-NNN --format json && docket show TKT-NNN --format json",
		AutomationHint: "Use --automation (or DOCKET_AUTOMATION=1) for deterministic non-interactive execution.",
	}
}
