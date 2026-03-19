package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/capabilities"
	"github.com/leomorpho/docket/internal/skills"
	"github.com/leomorpho/docket/internal/skillusage"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

type mockWorkflowRunner struct {
	startCalls  int
	finishCalls int
	startTicket *ticket.Ticket
	startPath   string
}

func (m *mockWorkflowRunner) StartTask(ctx context.Context, ticketID, agentID string, cfg *ticket.Config) (*ticket.Ticket, string, error) {
	m.startCalls++
	if m.startTicket != nil {
		return m.startTicket, m.startPath, nil
	}
	return &ticket.Ticket{ID: ticketID, State: "in-progress"}, m.startPath, nil
}

func (m *mockWorkflowRunner) FinishTask(ctx context.Context, ticketID string, cfg *ticket.Config) (*ticket.Ticket, error) {
	m.finishCalls++
	return &ticket.Ticket{ID: ticketID, State: "in-review"}, nil
}

type mockClaimLookup struct{}

func (m *mockClaimLookup) GetClaim(ctx context.Context, ticketID string) (*ClaimMetadata, error) {
	return nil, nil
}

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
		ID: "TKT-001", Seq: 1, Title: "Existing", State: ticket.State("todo"), Priority: 1,
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

func TestServeMCPWithDeps_DelegatesStartTask(t *testing.T) {
	repo := t.TempDir()
	if err := ticket.SaveConfig(repo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(repo)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "Existing", State: ticket.State("todo"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "d", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}

	mockWF := &mockWorkflowRunner{
		startTicket: &ticket.Ticket{ID: "TKT-001", State: "in-progress", Priority: 1},
		startPath:   "/tmp/wt-TKT-001",
	}
	deps := &DispatchDeps{
		RepoRoot: repo,
		Store:    s,
		Workflow: mockWF,
		Claimer:  &mockClaimLookup{},
		Config:   ticket.DefaultConfig(),
	}

	in := strings.NewReader(`{"id":1,"action":"update","args":{"id":"TKT-001","state":"in-progress"}}` + "\n")
	var out bytes.Buffer
	if err := ServeMCPWithDeps(in, &out, deps); err != nil {
		t.Fatalf("ServeMCPWithDeps failed: %v", err)
	}

	if mockWF.startCalls != 1 {
		t.Fatalf("StartTask calls = %d, want 1", mockWF.startCalls)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["ok"] != true {
		t.Fatalf("response not ok: %v", resp)
	}
}

func TestServeMCPWithDeps_DelegatesFinishTaskOnReview(t *testing.T) {
	repo := t.TempDir()
	if err := ticket.SaveConfig(repo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(repo)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "Existing", State: ticket.State("in-progress"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "d", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}

	mockWF := &mockWorkflowRunner{}
	deps := &DispatchDeps{
		RepoRoot: repo,
		Store:    s,
		Workflow: mockWF,
		Claimer:  &mockClaimLookup{},
		Config:   ticket.DefaultConfig(),
	}

	in := strings.NewReader(`{"id":1,"action":"update","args":{"id":"TKT-001","state":"in-review"}}` + "\n")
	var out bytes.Buffer
	if err := ServeMCPWithDeps(in, &out, deps); err != nil {
		t.Fatalf("ServeMCPWithDeps failed: %v", err)
	}

	if mockWF.finishCalls != 1 {
		t.Fatalf("FinishTask calls = %d, want 1", mockWF.finishCalls)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["ok"] != true {
		t.Fatalf("response not ok: %v", resp)
	}
	if resp["result"].(map[string]interface{})["state"] != "in-review" {
		t.Fatalf("expected response state in-review, got %v", resp["result"])
	}
}

func TestServeMCP_HookExecutionActionIsRejected(t *testing.T) {
	repo := t.TempDir()
	if err := ticket.SaveConfig(repo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	in := strings.NewReader(`{"id":1,"action":"hook.run","args":{"event":"ticket.review"}}` + "\n")
	var out bytes.Buffer
	if err := ServeMCP(in, &out, repo); err != nil {
		t.Fatalf("ServeMCP failed: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["ok"] != false {
		t.Fatalf("expected hook.run action to be rejected, got: %v", resp)
	}
	errText, _ := resp["error"].(string)
	if !strings.Contains(errText, "unknown action") {
		t.Fatalf("expected unknown action error, got: %q", errText)
	}
}

func TestServeMCP_HookMetadataAndStatus(t *testing.T) {
	repo := t.TempDir()
	if err := ticket.SaveConfig(repo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
	if _, _, err := capabilities.WriteRuntimeContract(repo, capabilities.CanonicalContractV1()); err != nil {
		t.Fatalf("write runtime contract failed: %v", err)
	}

	in := strings.NewReader(`{"id":1,"action":"hook.list"}
{"id":2,"action":"hook.show","args":{"id":"ticket.review"}}
{"id":3,"action":"hook.status"}
{"id":4,"action":"hook.show","args":{"id":"does-not-exist"}}
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

	if r1["ok"] != true || r2["ok"] != true || r3["ok"] != true {
		t.Fatalf("expected hook metadata/status actions to succeed, got r1=%v r2=%v r3=%v", r1, r2, r3)
	}
	listResult := r1["result"].(map[string]interface{})
	if listResult["execution"] != capabilities.HookExecutionInternal {
		t.Fatalf("expected internal-only hook execution mode, got %#v", listResult["execution"])
	}
	if listResult["ready"] != false || listResult["readiness"] != "needs-setup" {
		t.Fatalf("expected degraded readiness before hook install, got %#v", listResult)
	}
	events := listResult["events"].([]interface{})
	if len(events) == 0 {
		t.Fatalf("expected hook events in list result")
	}
	showResult := r2["result"].(map[string]interface{})
	event := showResult["event"].(map[string]interface{})
	if event["name"] != "ticket.review" {
		t.Fatalf("expected hook.show to return ticket.review, got %#v", event)
	}
	statusResult := r3["result"].(map[string]interface{})
	if statusResult["ready"] != false || statusResult["readiness"] != "needs-setup" {
		t.Fatalf("expected hook.status degraded before hook install, got %#v", statusResult)
	}
	if r4["ok"] != false || !strings.Contains(r4["error"].(string), "not found") {
		t.Fatalf("expected unknown hook.show to fail with not found, got %#v", r4)
	}

	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write hook failed: %v", err)
	}
	commitHookPath := filepath.Join(repo, ".git", "hooks", "commit-msg")
	if err := os.WriteFile(commitHookPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write commit-msg hook failed: %v", err)
	}

	inReady := strings.NewReader(`{"id":5,"action":"hook.status"}` + "\n")
	var outReady bytes.Buffer
	if err := ServeMCP(inReady, &outReady, repo); err != nil {
		t.Fatalf("ServeMCP (ready) failed: %v", err)
	}
	var r5 map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(outReady.Bytes()), &r5); err != nil {
		t.Fatalf("unmarshal ready response: %v", err)
	}
	if r5["ok"] != true {
		t.Fatalf("expected hook.status after install to succeed, got %#v", r5)
	}
	readyResult := r5["result"].(map[string]interface{})
	if readyResult["ready"] != true || readyResult["readiness"] != "ready" {
		t.Fatalf("expected ready hook.status after hook install, got %#v", readyResult)
	}
}

func TestServeMCP_CapabilitiesDiscoveryIncludesSkillMetadata(t *testing.T) {
	repo := t.TempDir()
	if err := ticket.SaveConfig(repo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "AGENTS.md"), []byte("codex marker"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md failed: %v", err)
	}
	runtime, _, err := capabilities.WriteRuntimeContract(repo, capabilities.CanonicalContractV1())
	if err != nil {
		t.Fatalf("write runtime contract failed: %v", err)
	}
	pack, report := skills.BuildPack(runtime)
	if !report.Valid() {
		t.Fatalf("build pack report invalid: %#v", report.Errors)
	}

	in := strings.NewReader(`{"id":1,"action":"capabilities"}` + "\n")
	var out bytes.Buffer
	if err := ServeMCP(in, &out, repo); err != nil {
		t.Fatalf("ServeMCP failed: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["ok"] != true {
		t.Fatalf("expected capabilities action to succeed, got: %v", resp)
	}
	result := resp["result"].(map[string]interface{})
	if result["skill_metadata_checksum"] != pack.MetadataChecksum {
		t.Fatalf("expected metadata checksum %s, got %v", pack.MetadataChecksum, result["skill_metadata_checksum"])
	}
	contract := result["contract"].(map[string]interface{})
	digest := result["capability_digest"].(map[string]interface{})
	if digest["adapter"] != result["adapter"].(map[string]interface{})["id"] {
		t.Fatalf("expected capability digest adapter alignment, got digest=%#v adapter=%#v", digest, result["adapter"])
	}
	flowPhases := digest["flow_phases"].([]interface{})
	contractPhases := contract["workflow"].(map[string]interface{})["phases"].([]interface{})
	if len(flowPhases) != len(contractPhases) {
		t.Fatalf("expected capability digest flow phase length %d, got %d", len(contractPhases), len(flowPhases))
	}
	for i := range flowPhases {
		if flowPhases[i] != contractPhases[i] {
			t.Fatalf("flow phase mismatch at index %d: digest=%v contract=%v", i, flowPhases[i], contractPhases[i])
		}
	}
	readiness := digest["readiness"].(map[string]interface{})
	if readiness["mcp"] == nil || readiness["skills"] == nil || readiness["hooks"] == nil {
		t.Fatalf("expected capability digest readiness fields, got %#v", readiness)
	}
	quickstart := result["agent_quickstart"].(map[string]interface{})
	if quickstart["direct_edit_avoidance"] == nil {
		t.Fatalf("expected quickstart direct_edit_avoidance guidance, got %#v", quickstart)
	}
	if quickstart["skills_reminder"] == nil {
		t.Fatalf("expected quickstart skills_reminder guidance, got %#v", quickstart)
	}
	capDiscovery := quickstart["capability_discovery"].([]interface{})
	if len(capDiscovery) == 0 {
		t.Fatalf("expected quickstart capability discovery commands, got %#v", quickstart)
	}
	foundSkillList := false
	for _, raw := range capDiscovery {
		if strings.Contains(raw.(string), "docket skill list --format json") {
			foundSkillList = true
			break
		}
	}
	if !foundSkillList {
		t.Fatalf("expected quickstart capability discovery to include skill list, got %#v", capDiscovery)
	}
	quickPath := result["llm_quick_path"].(map[string]interface{})
	if quickPath["ticket_apply"] == nil || quickPath["automation_hint"] == nil {
		t.Fatalf("expected llm_quick_path guidance fields, got %#v", quickPath)
	}

	skillsObj := contract["skills"].(map[string]interface{})
	inventory := skillsObj["inventory"].([]interface{})
	if len(inventory) == 0 {
		t.Fatalf("expected skills inventory in capabilities response")
	}
	firstSkill := inventory[0].(map[string]interface{})
	if firstSkill["title"] == nil || firstSkill["summary"] == nil || firstSkill["intent"] == nil || firstSkill["command"] == nil || firstSkill["triggers"] == nil {
		t.Fatalf("expected first-class skill metadata in capabilities response, got %#v", firstSkill)
	}
}

func TestServeMCP_SkillDiscoveryAndInvoke(t *testing.T) {
	repo := t.TempDir()
	if err := ticket.SaveConfig(repo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "AGENTS.md"), []byte("codex marker"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md failed: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	s := local.New(repo)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-964", Seq: 964, Title: "Learning replay target", State: ticket.State("todo"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "d", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if _, _, err := capabilities.WriteRuntimeContract(repo, capabilities.CanonicalContractV1()); err != nil {
		t.Fatalf("write runtime contract failed: %v", err)
	}

	in := strings.NewReader(`{"id":1,"action":"skill.list"}
{"id":2,"action":"skill.show","args":{"id":"learning-replay"}}
{"id":3,"action":"skill.invoke","args":{"id":"learning-replay","ticket_id":"TKT-964"}}
{"id":4,"action":"skill.invoke","args":{"id":"learning-replay"}}
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

	if r1["ok"] != true || r2["ok"] != true || r3["ok"] != true {
		t.Fatalf("expected skill list/show/invoke success, got r1=%v r2=%v r3=%v", r1, r2, r3)
	}
	cmd := r3["result"].(map[string]interface{})["command"].(string)
	if !strings.Contains(cmd, "docket learn replay TKT-964") {
		t.Fatalf("expected resolved skill command, got %q", cmd)
	}
	events, err := skillusage.Load(repo)
	if err != nil {
		t.Fatalf("load skill usage failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one recorded MCP skill usage event, got %#v", events)
	}
	if events[0].Source != skillusage.SourceMCP || events[0].SkillID != "learning-replay" {
		t.Fatalf("unexpected skill usage event: %#v", events[0])
	}
	if r4["ok"] != false {
		t.Fatalf("expected missing ticket_id invoke to fail, got %v", r4)
	}
	if !strings.Contains(r4["error"].(string), "requires ticket_id") {
		t.Fatalf("expected missing ticket_id error, got %v", r4["error"])
	}
}

func TestServeMCP_DiscoveryFlowsEndToEnd(t *testing.T) {
	repo := t.TempDir()
	if err := ticket.SaveConfig(repo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "hooks", "pre-commit"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write pre-commit failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "AGENTS.md"), []byte("codex marker"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md failed: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	s := local.New(repo)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-120", Seq: 120, Title: "MCP discovery fixture", State: ticket.State("todo"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "d", AC: []ticket.AcceptanceCriterion{{Description: "x"}},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if _, _, err := capabilities.WriteRuntimeContract(repo, capabilities.CanonicalContractV1()); err != nil {
		t.Fatalf("write runtime contract failed: %v", err)
	}

	in := strings.NewReader(`{"id":"cap","action":"capabilities"}
{"id":"sl","action":"skill.list"}
{"id":"ss","action":"skill.show","args":{"id":"learning-replay"}}
{"id":"si","action":"skill.invoke","args":{"id":"learning-replay","ticket_id":"TKT-120"}}
{"id":"hl","action":"hook.list"}
{"id":"hs","action":"hook.status"}
`)
	var out bytes.Buffer
	if err := ServeMCP(in, &out, repo); err != nil {
		t.Fatalf("ServeMCP failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("responses = %d, want 6\n%s", len(lines), out.String())
	}

	byID := map[string]map[string]interface{}{}
	for _, line := range lines {
		var resp map[string]interface{}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("unmarshal response failed: %v\n%s", err, line)
		}
		key, _ := resp["id"].(string)
		byID[key] = resp
	}

	for _, id := range []string{"cap", "sl", "ss", "si", "hl", "hs"} {
		resp := byID[id]
		if resp["ok"] != true {
			t.Fatalf("expected %s to succeed, got %#v", id, resp)
		}
	}

	capResult := byID["cap"]["result"].(map[string]interface{})
	if capResult["contract"] == nil {
		t.Fatalf("expected capabilities contract payload, got %#v", capResult)
	}
	skillInvokeCmd := byID["si"]["result"].(map[string]interface{})["command"].(string)
	if !strings.Contains(skillInvokeCmd, "docket learn replay TKT-120") {
		t.Fatalf("expected resolved invoke command with ticket ID, got %q", skillInvokeCmd)
	}
	hookList := byID["hl"]["result"].(map[string]interface{})
	events := hookList["events"].([]interface{})
	if len(events) == 0 {
		t.Fatalf("expected hook events in hook.list result")
	}
	hookStatus := byID["hs"]["result"].(map[string]interface{})
	if hookStatus["readiness"] == nil || hookStatus["ready"] == nil {
		t.Fatalf("expected hook readiness fields in hook.status, got %#v", hookStatus)
	}
}

func TestServeMCP_DiscoveryErrorResponsesAreMachineReadable(t *testing.T) {
	repo := t.TempDir()
	if err := ticket.SaveConfig(repo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
	if _, _, err := capabilities.WriteRuntimeContract(repo, capabilities.CanonicalContractV1()); err != nil {
		t.Fatalf("write runtime contract failed: %v", err)
	}

	in := strings.NewReader(`{"id":1,"action":"unknown.action"}
{"id":2,"action":"skill.show","args":{}}
{"id":3,"action":"skill.invoke","args":{"id":"learning-replay"}}
{"id":4,"action":"hook.show","args":{}}
{"id":5,"action":"hook.show","args":{"id":"missing.event"}}
`)
	var out bytes.Buffer
	if err := ServeMCP(in, &out, repo); err != nil {
		t.Fatalf("ServeMCP failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("responses = %d, want 5\n%s", len(lines), out.String())
	}

	expectErrContains := []string{
		"unknown action",
		"id is required",
		"requires ticket_id",
		"id is required",
		"not found",
	}

	for i, line := range lines {
		var resp map[string]interface{}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("unmarshal response %d failed: %v\n%s", i+1, err, line)
		}
		if resp["ok"] != false {
			t.Fatalf("expected response %d to be error, got %#v", i+1, resp)
		}
		errText, ok := resp["error"].(string)
		if !ok || strings.TrimSpace(errText) == "" {
			t.Fatalf("expected response %d to include machine-readable error text, got %#v", i+1, resp)
		}
		if !strings.Contains(errText, expectErrContains[i]) {
			t.Fatalf("response %d error %q does not contain %q", i+1, errText, expectErrContains[i])
		}
	}
}
