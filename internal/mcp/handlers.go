package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
