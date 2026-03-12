package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestServeMCP_ListCreateAndUnknown(t *testing.T) {
	repo := t.TempDir()
	if err := ticket.SaveConfig(repo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(repo)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-099", Seq: 99, Title: "Existing", State: ticket.State("todo"), Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "d", AC: []ticket.AcceptanceCriterion{{Description: "x"}}}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}

	in := strings.NewReader(`{"id":1,"action":"list","args":{"state":"open"}}
{"id":2,"action":"create","args":{"title":"New from mcp"}}
{"id":3,"action":"unknown"}
`)
	var out bytes.Buffer
	if err := ServeMCP(in, &out, repo); err != nil {
		t.Fatalf("ServeMCP failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("responses = %d, want 3\n%s", len(lines), out.String())
	}

	var r1, r2, r3 map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &r1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &r2); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[2]), &r3); err != nil {
		t.Fatal(err)
	}

	if r1["ok"] != true {
		t.Fatalf("list response not ok: %v", r1)
	}
	if r2["ok"] != true {
		t.Fatalf("create response not ok: %v", r2)
	}
	if r3["ok"] != false {
		t.Fatalf("unknown response should be error: %v", r3)
	}
}

func TestServeMCP_ShowUpdateCommentAndInvalidTransition(t *testing.T) {
	repo := t.TempDir()
	if err := ticket.SaveConfig(repo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(repo)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "Existing", State: ticket.State("backlog"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "d", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}

	in := strings.NewReader(`{"id":1,"action":"show","args":{"id":"TKT-001"}}
{"id":2,"action":"update","args":{"id":"TKT-001","state":"done"}}
{"id":3,"action":"comment","args":{"id":"TKT-001","body":"hello"}}
{"id":4,"action":"update","args":{"id":"TKT-001","state":"todo"}}
`)
	var out bytes.Buffer
	if err := ServeMCP(in, &out, repo); err != nil {
		t.Fatalf("ServeMCP failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("responses = %d, want 4\n%s", len(lines), out.String())
	}

	var r1, r2, r3, r4 map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &r1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &r2); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[2]), &r3); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[3]), &r4); err != nil {
		t.Fatal(err)
	}

	if r1["ok"] != true {
		t.Fatalf("show response not ok: %v", r1)
	}
	if r2["ok"] != false {
		t.Fatalf("invalid transition should fail: %v", r2)
	}
	if r3["ok"] != true {
		t.Fatalf("comment response not ok: %v", r3)
	}
	if r4["ok"] != true {
		t.Fatalf("valid transition should pass: %v", r4)
	}
}
