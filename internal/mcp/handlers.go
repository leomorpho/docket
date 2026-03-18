package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/adapters"
	"github.com/leomorpho/docket/internal/capabilities"
	"github.com/leomorpho/docket/internal/skills"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
)

type WorkflowRunner interface {
	StartTask(ctx context.Context, ticketID, agentID string, cfg *ticket.Config) (*ticket.Ticket, string, error)
	FinishTask(ctx context.Context, ticketID string, cfg *ticket.Config) (*ticket.Ticket, error)
}

type ClaimMetadata struct {
	AgentID  string
	Worktree string
}

type ClaimLookup interface {
	GetClaim(ctx context.Context, ticketID string) (*ClaimMetadata, error)
}

type DispatchDeps struct {
	RepoRoot string
	Store    store.Backend
	Workflow WorkflowRunner
	Claimer  ClaimLookup
	Config   *ticket.Config
}

func Dispatch(action string, args map[string]interface{}, deps *DispatchDeps) (interface{}, error) {
	if deps == nil || deps.Store == nil || deps.Workflow == nil || deps.Claimer == nil || deps.Config == nil {
		return nil, fmt.Errorf("mcp dependencies not initialized")
	}

	action = strings.TrimSpace(strings.ToLower(action))
	ctx := context.Background()
	cfg := deps.Config

	switch action {
	case "list":
		return handleList(ctx, deps.Store, deps.Claimer, cfg, args)
	case "create":
		return handleCreate(ctx, deps.Store, cfg, args, deps.RepoRoot)
	case "show":
		return handleShow(ctx, deps.Store, args)
	case "update":
		return handleUpdate(ctx, deps.Store, deps.Workflow, cfg, args, deps.RepoRoot)
	case "comment":
		return handleComment(ctx, deps.Store, args)
	case "capabilities":
		return handleCapabilities(deps.RepoRoot)
	case "skill.list":
		return handleSkillList(deps.RepoRoot)
	case "skill.show":
		return handleSkillShow(deps.RepoRoot, args)
	case "skill.invoke":
		return handleSkillInvoke(ctx, deps.RepoRoot, deps.Store, args)
	case "hook.list":
		return handleHookList(deps.RepoRoot)
	case "hook.show":
		return handleHookShow(deps.RepoRoot, args)
	case "hook.status":
		return handleHookStatus(deps.RepoRoot)
	case "check":
		// Return human/check command users can call through CLI for now.
		return map[string]interface{}{"message": "use CLI for full check output"}, nil
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func getContent(args map[string]interface{}, textKey, fileKey, repoRoot string) (string, bool) {
	if filePath, ok := getString(args, fileKey); ok && filePath != "" {
		// Security: Validate path is within repo or /tmp
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return "", false
		}

		absRepo, _ := filepath.Abs(repoRoot)
		inRepo := strings.HasPrefix(absPath, absRepo)
		inTmp := strings.HasPrefix(absPath, "/tmp") || strings.HasPrefix(absPath, os.TempDir())

		if !inRepo && !inTmp {
			return "", false
		}

		data, err := os.ReadFile(absPath)
		if err == nil {
			return string(data), true
		}
	}
	return getString(args, textKey)
}

func handleList(
	ctx context.Context,
	s store.Backend,
	claimMgr ClaimLookup,
	cfg *ticket.Config,
	args map[string]interface{},
) (interface{}, error) {
	var openStates []ticket.State
	for _, st := range cfg.OpenStates() {
		openStates = append(openStates, ticket.State(st))
	}
	f := store.Filter{States: openStates}

	if state, ok := getString(args, "state"); ok && !strings.EqualFold(state, "open") {
		if !cfg.IsValidState(state) {
			return nil, fmt.Errorf("invalid state: %s", state)
		}
		st := ticket.State(state)
		f = store.Filter{States: []ticket.State{st}, IncludeArchived: state == "archived"}
	}

	tickets, err := s.ListTickets(ctx, f)
	if err != nil {
		return nil, err
	}

	type ticketResp struct {
		*ticket.Ticket
		ClaimedBy string `json:"claimed_by,omitempty"`
	}

	var resp []ticketResp
	for _, t := range tickets {
		cl, _ := claimMgr.GetClaim(ctx, t.ID)
		claimedBy := ""
		if cl != nil {
			claimedBy = cl.AgentID
		}
		resp = append(resp, ticketResp{Ticket: t, ClaimedBy: claimedBy})
	}

	return resp, nil
}

func handleCreate(ctx context.Context, s store.Backend, cfg *ticket.Config, args map[string]interface{}, repoRoot string) (interface{}, error) {
	title, ok := getString(args, "title")
	if !ok || strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("title is required")
	}
	id, seq, err := s.NextID(ctx)
	if err != nil {
		return nil, err
	}
	state := cfg.DefaultState
	if v, ok := getString(args, "state"); ok && v != "" {
		state = v
	}
	if !cfg.IsValidState(state) {
		return nil, fmt.Errorf("invalid state: %s", state)
	}
	priority := cfg.DefaultPriority
	if p, ok := getInt(args, "priority"); ok {
		priority = p
	}
	desc, _ := getContent(args, "desc", "content_file", repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	t := &ticket.Ticket{ID: id, Seq: seq, Title: title, Description: desc, State: ticket.State(state), Priority: priority, CreatedAt: now, UpdatedAt: now, CreatedBy: "agent:mcp"}
	if err := s.CreateTicket(ctx, t); err != nil {
		return nil, err
	}
	return map[string]interface{}{"id": t.ID, "seq": t.Seq, "title": t.Title, "state": t.State}, nil
}

func handleShow(ctx context.Context, s store.Backend, args map[string]interface{}) (interface{}, error) {
	id, ok := getString(args, "id")
	if !ok || id == "" {
		return nil, fmt.Errorf("id is required")
	}
	t, err := s.GetTicket(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("ticket %s not found", id)
	}
	return t, nil
}

func handleUpdate(
	ctx context.Context,
	s store.Backend,
	wf WorkflowRunner,
	cfg *ticket.Config,
	args map[string]interface{},
	repoRoot string,
) (interface{}, error) {
	id, ok := getString(args, "id")
	if !ok || id == "" {
		return nil, fmt.Errorf("id is required")
	}
	t, err := s.GetTicket(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("ticket %s not found", id)
	}

	oldState := t.State
	lifecycleState := ticket.State("")
	if v, ok := getString(args, "state"); ok && v != "" {
		ns := ticket.State(v)
		if !cfg.IsValidState(v) {
			return nil, fmt.Errorf("invalid state: %s", v)
		}
		if err := ticket.ValidateTransition(cfg, t.State, ns); err != nil {
			return nil, err
		}
		switch ns {
		case "in-progress", "in-review":
			lifecycleState = ns
		default:
			t.State = ns
			if ns == "done" && t.CompletedAt.IsZero() {
				t.CompletedAt = time.Now().UTC().Truncate(time.Second)
			}
		}
	}

	if v, ok := getString(args, "title"); ok && strings.TrimSpace(v) != "" {
		t.Title = strings.TrimSpace(v)
	}
	if v, ok := getInt(args, "priority"); ok {
		t.Priority = v
	}
	if v, ok := getContent(args, "desc", "content_file", repoRoot); ok {
		t.Description = v
	}
	if v, ok := getString(args, "handoff"); ok {
		t.Handoff = v
	}

	t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if err := s.UpdateTicket(ctx, t); err != nil {
		return nil, err
	}

	resp := map[string]interface{}{"id": t.ID, "state": t.State, "priority": t.Priority}

	if lifecycleState == "in-progress" && oldState != "in-progress" {
		// Auto-claim
		actor := "agent:mcp"
		// If caller provided an agent ID via metadata
		if aid, ok := getString(args, "_agent_id"); ok {
			actor = "agent:" + aid
		}

		res, wtPath, err := wf.StartTask(ctx, t.ID, actor, cfg)
		if err != nil {
			return nil, err
		}
		t = res
		resp["state"] = t.State
		resp["priority"] = t.Priority
		if wtPath != repoRoot {
			resp["new_worktree_path"] = wtPath
		}
	} else if lifecycleState == "in-review" && oldState != "in-review" {
		res, err := wf.FinishTask(ctx, t.ID, cfg)
		if err != nil {
			return nil, err
		}
		t = res
		resp["state"] = t.State
		resp["priority"] = t.Priority
	}

	return resp, nil
}

func handleComment(ctx context.Context, s store.Backend, args map[string]interface{}) (interface{}, error) {
	id, ok := getString(args, "id")
	if !ok || id == "" {
		return nil, fmt.Errorf("id is required")
	}
	body, ok := getString(args, "body")
	if !ok || strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("body is required")
	}
	c := ticket.Comment{At: time.Now().UTC().Truncate(time.Second), Author: "agent:mcp", Body: body}
	if err := s.AddComment(ctx, id, c); err != nil {
		return nil, err
	}
	return map[string]interface{}{"ticket_id": id, "at": c.At.Format(time.RFC3339)}, nil
}

func handleCapabilities(repoRoot string) (interface{}, error) {
	runtime, _, err := capabilities.EnsureRuntimeContract(repoRoot)
	if err != nil {
		return nil, err
	}
	resolution := adapters.DefaultRegistry().ResolveWithInfo(repoRoot, "")
	pack, report := skills.BuildPack(runtime)
	if !report.Valid() {
		return nil, fmt.Errorf("invalid skill metadata in runtime contract: %#v", report.Errors)
	}
	capabilityDigest := buildCapabilityDigestView(repoRoot, resolution.Adapter.Metadata().ID)
	return map[string]interface{}{
		"adapter": map[string]any{
			"id":           resolution.Adapter.Metadata().ID,
			"display_name": resolution.Adapter.Metadata().DisplayName,
			"source":       resolution.Source,
		},
		"contract":                runtime,
		"skill_metadata_checksum": pack.MetadataChecksum,
		"agent_quickstart":        buildAgentQuickstartView(),
		"llm_quick_path":          buildQuickPathView(),
		"capability_digest":       capabilityDigest,
	}, nil
}

type mcpSkillEntry struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	Intent   string   `json:"intent"`
	Command  string   `json:"command"`
	Triggers []string `json:"triggers"`
	Optional bool     `json:"optional"`
}

func loadSkillCatalog(repoRoot string) ([]mcpSkillEntry, string, error) {
	runtime, _, err := capabilities.EnsureRuntimeContract(repoRoot)
	if err != nil {
		return nil, "", err
	}
	pack, report := skills.BuildPack(runtime)
	if !report.Valid() {
		return nil, "", fmt.Errorf("invalid skill metadata in runtime contract: %#v", report.Errors)
	}
	out := make([]mcpSkillEntry, 0, len(pack.Skills))
	for _, meta := range pack.Skills {
		out = append(out, mcpSkillEntry{
			ID:       meta.ID,
			Title:    meta.Title,
			Summary:  meta.Summary,
			Intent:   meta.Intent,
			Command:  meta.Command,
			Triggers: append([]string{}, meta.Triggers...),
			Optional: meta.Optional,
		})
	}
	return out, pack.MetadataChecksum, nil
}

func handleSkillList(repoRoot string) (interface{}, error) {
	items, checksum, err := loadSkillCatalog(repoRoot)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"total":             len(items),
		"metadata_checksum": checksum,
		"skills":            items,
	}, nil
}

func handleSkillShow(repoRoot string, args map[string]interface{}) (interface{}, error) {
	id, ok := getString(args, "id")
	if !ok || strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("id is required")
	}
	items, checksum, err := loadSkillCatalog(repoRoot)
	if err != nil {
		return nil, err
	}
	target := strings.ToLower(strings.TrimSpace(id))
	for _, item := range items {
		if strings.ToLower(item.ID) == target {
			return map[string]interface{}{
				"skill":             item,
				"metadata_checksum": checksum,
			}, nil
		}
	}
	return nil, fmt.Errorf("skill %s not found", strings.TrimSpace(id))
}

func handleSkillInvoke(ctx context.Context, repoRoot string, s store.Backend, args map[string]interface{}) (interface{}, error) {
	id, ok := getString(args, "id")
	if !ok || strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("id is required")
	}
	items, _, err := loadSkillCatalog(repoRoot)
	if err != nil {
		return nil, err
	}
	target := strings.ToLower(strings.TrimSpace(id))
	ticketID := strings.TrimSpace(getStringOr(args, "ticket_id", ""))
	if normalized, ok := ticket.NormalizeID(ticketID); ok {
		ticketID = normalized
	}
	for _, item := range items {
		if strings.ToLower(item.ID) != target {
			continue
		}
		command, err := resolveSkillInvokeCommand(item.Command, ticketID)
		if err != nil {
			return nil, err
		}
		if ticketID != "" {
			t, err := s.GetTicket(ctx, ticketID)
			if err != nil {
				return nil, err
			}
			if t == nil {
				return nil, fmt.Errorf("ticket %s not found", ticketID)
			}
		}
		return map[string]interface{}{
			"skill_id":  item.ID,
			"ticket_id": ticketID,
			"command":   command,
			"intent":    item.Intent,
			"summary":   item.Summary,
		}, nil
	}
	return nil, fmt.Errorf("skill %s not found", strings.TrimSpace(id))
}

func resolveSkillInvokeCommand(template, ticketID string) (string, error) {
	command := strings.TrimSpace(template)
	if command == "" {
		return "", fmt.Errorf("skill command template is empty")
	}
	if strings.Contains(command, "{ticket_id}") {
		if strings.TrimSpace(ticketID) == "" {
			return "", fmt.Errorf("this skill requires ticket_id")
		}
		command = strings.ReplaceAll(command, "{ticket_id}", ticketID)
	}
	return command, nil
}

type mcpHookEventEntry struct {
	Name     string `json:"name"`
	Mode     string `json:"mode"`
	Blocking bool   `json:"blocking"`
}

type mcpHookStatusView struct {
	Namespace  string              `json:"namespace"`
	Invocation string              `json:"invocation"`
	Execution  string              `json:"execution"`
	Ready      bool                `json:"ready"`
	Readiness  string              `json:"readiness"`
	Events     []mcpHookEventEntry `json:"events"`
}

func handleHookList(repoRoot string) (interface{}, error) {
	status, err := loadHookStatus(repoRoot)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"total":      len(status.Events),
		"events":     status.Events,
		"namespace":  status.Namespace,
		"invocation": status.Invocation,
		"execution":  status.Execution,
		"ready":      status.Ready,
		"readiness":  status.Readiness,
	}, nil
}

func handleHookShow(repoRoot string, args map[string]interface{}) (interface{}, error) {
	id := strings.TrimSpace(getStringOr(args, "id", ""))
	if id == "" {
		id = strings.TrimSpace(getStringOr(args, "event", ""))
	}
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	status, err := loadHookStatus(repoRoot)
	if err != nil {
		return nil, err
	}
	target := strings.ToLower(id)
	for _, event := range status.Events {
		if strings.ToLower(event.Name) != target {
			continue
		}
		return map[string]interface{}{
			"event":      event,
			"namespace":  status.Namespace,
			"invocation": status.Invocation,
			"execution":  status.Execution,
			"ready":      status.Ready,
			"readiness":  status.Readiness,
		}, nil
	}
	return nil, fmt.Errorf("hook event %s not found", id)
}

func handleHookStatus(repoRoot string) (interface{}, error) {
	status, err := loadHookStatus(repoRoot)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func loadHookStatus(repoRoot string) (mcpHookStatusView, error) {
	runtime, _, err := capabilities.EnsureRuntimeContract(repoRoot)
	if err != nil {
		return mcpHookStatusView{}, err
	}
	events := make([]mcpHookEventEntry, 0, len(runtime.Hooks.Events))
	for _, event := range runtime.Hooks.Events {
		events = append(events, mcpHookEventEntry{
			Name:     event.Name,
			Mode:     event.Mode,
			Blocking: event.Blocking,
		})
	}

	ready := isHookInstalled(repoRoot)
	readiness := "needs-setup"
	if ready {
		readiness = "ready"
	}
	return mcpHookStatusView{
		Namespace:  runtime.Hooks.Namespace,
		Invocation: runtime.Hooks.Invocation,
		Execution:  runtime.Hooks.Execution,
		Ready:      ready,
		Readiness:  readiness,
		Events:     events,
	}, nil
}

func isHookInstalled(repoRoot string) bool {
	paths := []string{
		filepath.Join(repoRoot, ".git", "hooks", "pre-commit"),
		filepath.Join(repoRoot, ".git", "hooks", "commit-msg"),
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return false
		}
	}
	return true
}

type capabilityDigestReadinessView struct {
	MCP    string `json:"mcp"`
	Skills string `json:"skills"`
	Hooks  string `json:"hooks"`
}

type capabilityDigestView struct {
	Adapter     string                        `json:"adapter"`
	FlowPhases  []string                      `json:"flow_phases"`
	Readiness   capabilityDigestReadinessView `json:"readiness"`
	Remediation string                        `json:"remediation,omitempty"`
}

func buildCapabilityDigestView(repoRoot, adapterID string) capabilityDigestView {
	if strings.TrimSpace(adapterID) == "" {
		adapterID = "unknown"
	}
	mcpReady := isMCPConfigured(repoRoot)
	skillsReady := isSkillGuidanceInstalled(repoRoot, adapterID)
	hooksReady := isHookInstalled(repoRoot)
	out := capabilityDigestView{
		Adapter:    adapterID,
		FlowPhases: []string{"bootstrap", "start", "plan", "implement", "verify"},
		Readiness: capabilityDigestReadinessView{
			MCP:    readinessLabel(mcpReady),
			Skills: readinessLabel(skillsReady),
			Hooks:  readinessLabel(hooksReady),
		},
	}
	if !(mcpReady && skillsReady && hooksReady) {
		out.Remediation = "Run `docket bootstrap` to install or repair integration artifacts."
	}
	return out
}

func readinessLabel(ok bool) string {
	if ok {
		return "ready"
	}
	return "needs-setup"
}

func isMCPConfigured(repoRoot string) bool {
	if fileExists(filepath.Join(repoRoot, "doombox.json")) {
		return true
	}
	if fileContains(filepath.Join(repoRoot, ".vscode", "settings.json"), "docket") {
		return true
	}
	if fileContains(filepath.Join(repoRoot, ".cursor", "mcp.json"), "docket") {
		return true
	}
	return false
}

func isSkillGuidanceInstalled(repoRoot, adapterID string) bool {
	switch adapterID {
	case "codex":
		return fileExists(filepath.Join(repoRoot, "AGENTS.md"))
	case "claude-code":
		return fileExists(filepath.Join(repoRoot, "CLAUDE.md"))
	case "gemini":
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		return fileExists(filepath.Join(home, ".gemini", "skills", "docket", "SKILL.md"))
	default:
		return false
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileContains(path, needle string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), needle)
}

func buildAgentQuickstartView() map[string]any {
	return map[string]any{
		"direct_edit_avoidance": "Never edit .docket/tickets/*.md directly; use `docket` commands so ticket signatures and metadata remain valid.",
		"core_workflow": []string{
			"docket list --state open --format context",
			"docket show TKT-NNN --format context",
			"docket update TKT-NNN --state in-progress",
			"docket ac check TKT-NNN",
		},
		"capability_discovery": []string{
			"docket capabilities --format json",
			"docket doctor --format json",
			"docket help-json",
		},
	}
}

func buildQuickPathView() map[string]any {
	return map[string]any{
		"preference":      "Prefer transactional scaffold/apply commands over multi-step manual create/update flows.",
		"ticket_apply":    "docket ticket scaffold > ticket-spec.json && docket --automation ticket apply --spec ticket-spec.json",
		"backlog_apply":   "docket backlog scaffold > backlog-spec.json && docket --automation backlog apply --spec backlog-spec.json",
		"proof_attach":    "docket proof add TKT-NNN --file artifacts/screenshot.png --proof-title \"Before fix\" --note \"What this screenshot proves\" --captured-at 2026-03-16T18:40:00Z --format json",
		"proof_verify":    "docket proof list TKT-NNN --format json && docket show TKT-NNN --format json",
		"automation_hint": "Use --automation (or DOCKET_AUTOMATION=1) for deterministic non-interactive execution.",
	}
}

func getString(m map[string]interface{}, key string) (string, bool) {
	if m == nil {
		return "", false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func getStringOr(m map[string]interface{}, key, def string) string {
	if v, ok := getString(m, key); ok {
		return v
	}
	return def
}

func getInt(m map[string]interface{}, key string) (int, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		p, err := strconv.Atoi(n)
		if err == nil {
			return p, true
		}
	}
	return 0, false
}
