package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/leoaudibert/docket/internal/store"
	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
)

func Dispatch(action string, args map[string]interface{}, repoRoot string) (interface{}, error) {
	action = strings.TrimSpace(strings.ToLower(action))
	s := local.New(repoRoot)
	ctx := context.Background()

	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	switch action {
	case "list":
		return handleList(ctx, s, cfg, args)
	case "create":
		return handleCreate(ctx, s, cfg, args)
	case "show":
		return handleShow(ctx, s, args)
	case "update":
		return handleUpdate(ctx, s, cfg, args)
	case "comment":
		return handleComment(ctx, s, args)
	case "check":
		// Return human/check command users can call through CLI for now.
		return map[string]interface{}{"message": "use CLI for full check output"}, nil
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func handleList(ctx context.Context, s *local.Store, cfg *ticket.Config, args map[string]interface{}) (interface{}, error) {
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
	return s.ListTickets(ctx, f)
}

func handleCreate(ctx context.Context, s *local.Store, cfg *ticket.Config, args map[string]interface{}) (interface{}, error) {
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
	now := time.Now().UTC().Truncate(time.Second)
	t := &ticket.Ticket{ID: id, Seq: seq, Title: title, Description: getStringOr(args, "desc", ""), State: ticket.State(state), Priority: priority, CreatedAt: now, UpdatedAt: now, CreatedBy: "agent:mcp"}
	if err := s.CreateTicket(ctx, t); err != nil {
		return nil, err
	}
	return map[string]interface{}{"id": t.ID, "seq": t.Seq, "title": t.Title, "state": t.State}, nil
}

func handleShow(ctx context.Context, s *local.Store, args map[string]interface{}) (interface{}, error) {
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

func handleUpdate(ctx context.Context, s *local.Store, cfg *ticket.Config, args map[string]interface{}) (interface{}, error) {
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
	if v, ok := getString(args, "state"); ok && v != "" {
		ns := ticket.State(v)
		if !cfg.IsValidState(v) {
			return nil, fmt.Errorf("invalid state: %s", v)
		}
		if err := ticket.ValidateTransition(cfg, t.State, ns); err != nil {
			return nil, err
		}
		t.State = ns
	}
	if v, ok := getString(args, "title"); ok && strings.TrimSpace(v) != "" {
		t.Title = strings.TrimSpace(v)
	}
	if v, ok := getInt(args, "priority"); ok {
		t.Priority = v
	}
	t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if err := s.UpdateTicket(ctx, t); err != nil {
		return nil, err
	}
	return map[string]interface{}{"id": t.ID, "state": t.State, "priority": t.Priority}, nil
}

func handleComment(ctx context.Context, s *local.Store, args map[string]interface{}) (interface{}, error) {
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
