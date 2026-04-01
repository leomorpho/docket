package cmd

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/capabilities"
	"github.com/leomorpho/docket/internal/skills"
	"github.com/leomorpho/docket/internal/skillusage"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestSkillListAndShowStayAlignedWithCanonicalMetadata(t *testing.T) {
	h := newFakeRepoHarness(t)

	out, err := h.run("skill", "list", "--format", "json")
	if err != nil {
		t.Fatalf("skill list failed: %v\n%s", err, out)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal skill list json failed: %v\n%s", err, out)
	}
	entries, ok := payload["skills"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("expected non-empty skills list, got %#v", payload["skills"])
	}

	runtime, _, err := capabilities.EnsureRuntimeContract(h.repo)
	if err != nil {
		t.Fatalf("ensure runtime contract failed: %v", err)
	}
	pack, report := skills.BuildPack(runtime)
	if !report.Valid() {
		t.Fatalf("expected valid skill metadata in runtime contract: %#v", report.Errors)
	}
	if payload["metadata_checksum"] != pack.MetadataChecksum {
		t.Fatalf("expected metadata checksum %s, got %#v", pack.MetadataChecksum, payload["metadata_checksum"])
	}

	gotIDs := make([]string, 0, len(entries))
	for _, raw := range entries {
		entry := raw.(map[string]any)
		if entry["id"] == nil || entry["title"] == nil || entry["summary"] == nil || entry["intent"] == nil || entry["command"] == nil || entry["triggers"] == nil {
			t.Fatalf("expected first-class skill metadata fields, got %#v", entry)
		}
		gotIDs = append(gotIDs, entry["id"].(string))
	}
	wantIDs := skills.ContractSkillIDs(runtime)
	sort.Strings(gotIDs)
	sort.Strings(wantIDs)
	if strings.Join(gotIDs, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("expected skill ids from list to match runtime contract, got=%v want=%v", gotIDs, wantIDs)
	}

	firstID := entries[0].(map[string]any)["id"].(string)
	showOut, err := h.run("skill", "show", firstID, "--format", "json")
	if err != nil {
		t.Fatalf("skill show failed: %v\n%s", err, showOut)
	}
	var show map[string]any
	if err := json.Unmarshal([]byte(showOut), &show); err != nil {
		t.Fatalf("unmarshal skill show json failed: %v\n%s", err, showOut)
	}
	skillObj := show["skill"].(map[string]any)
	if skillObj["id"] != firstID {
		t.Fatalf("expected shown skill id %s, got %#v", firstID, skillObj["id"])
	}
	if show["metadata_checksum"] != pack.MetadataChecksum {
		t.Fatalf("expected show metadata checksum %s, got %#v", pack.MetadataChecksum, show["metadata_checksum"])
	}
}

func TestSkillInvokeResolvesCommandsAndValidatesInputs(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-964", 964, ticket.State("draft"), []ticket.AcceptanceCriterion{{Description: "ac"}})

	invokeOut, err := h.run("skill", "invoke", "learning-replay", "--ticket", "TKT-964", "--format", "json")
	if err != nil {
		t.Fatalf("skill invoke failed: %v\n%s", err, invokeOut)
	}
	var invoke map[string]any
	if err := json.Unmarshal([]byte(invokeOut), &invoke); err != nil {
		t.Fatalf("unmarshal skill invoke json failed: %v\n%s", err, invokeOut)
	}
	if invoke["skill_id"] != "learning-replay" {
		t.Fatalf("expected learning-replay skill id, got %#v", invoke["skill_id"])
	}
	command := invoke["command"].(string)
	if !strings.Contains(command, "docket learn replay TKT-964") {
		t.Fatalf("expected resolved learning replay command, got %s", command)
	}

	hMissing := newFakeRepoHarness(t)
	skillInvokeTicket = ""
	_ = skillInvokeCmd.Flags().Set("ticket", "")
	if f := skillInvokeCmd.Flags().Lookup("ticket"); f != nil {
		f.Changed = false
	}
	missingTicketOut, err := hMissing.run("skill", "invoke", "learning-replay")
	if err == nil {
		t.Fatalf("expected skill invoke without ticket to fail, output=%s", missingTicketOut)
	}
	if !strings.Contains(missingTicketOut, "requires --ticket") {
		t.Fatalf("expected missing ticket validation error, got:\n%s", missingTicketOut)
	}

	hUnknown := newFakeRepoHarness(t)
	unknownOut, err := hUnknown.run("skill", "invoke", "not-a-skill")
	if err == nil {
		t.Fatalf("expected unknown skill invoke to fail, output=%s", unknownOut)
	}
	if !strings.Contains(unknownOut, "skill not-a-skill not found") {
		t.Fatalf("expected unknown skill error, got:\n%s", unknownOut)
	}
}

func TestSkillAuditReportsRecordedUsage(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-301", 301, ticket.State("draft"), []ticket.AcceptanceCriterion{{Description: "ac"}})

	originalNow := skillusage.Now
	defer func() { skillusage.Now = originalNow }()
	stamps := []string{
		"2026-03-01T10:00:00Z",
		"2026-03-02T11:00:00Z",
	}
	idx := 0
	skillusage.Now = func() time.Time {
		ts := stamps[idx]
		if idx < len(stamps)-1 {
			idx++
		}
		parsed, _ := time.Parse(time.RFC3339, ts)
		return parsed
	}

	if _, err := h.run("skill", "invoke", "learning-replay", "--ticket", "TKT-301"); err != nil {
		t.Fatalf("first invoke failed: %v", err)
	}
	if _, err := h.run("skill", "invoke", "learning-replay", "--ticket", "TKT-301"); err != nil {
		t.Fatalf("second invoke failed: %v", err)
	}

	out, err := h.run("skill", "audit", "--format", "json")
	if err != nil {
		t.Fatalf("skill audit failed: %v\n%s", err, out)
	}
	var audit map[string]any
	if err := json.Unmarshal([]byte(out), &audit); err != nil {
		t.Fatalf("unmarshal skill audit json failed: %v\n%s", err, out)
	}
	if audit["total_invocations"] != float64(2) {
		t.Fatalf("expected 2 invocations, got %#v", audit["total_invocations"])
	}
	if audit["bucket_size"] != "day" {
		t.Fatalf("expected day bucket, got %#v", audit["bucket_size"])
	}
	skillsList := audit["skills"].([]any)
	if len(skillsList) != 1 {
		t.Fatalf("expected 1 skill entry, got %#v", skillsList)
	}
	skillEntry := skillsList[0].(map[string]any)
	if skillEntry["id"] != "learning-replay" || skillEntry["count"] != float64(2) {
		t.Fatalf("unexpected skill entry: %#v", skillEntry)
	}
	timeline := audit["timeline"].([]any)
	if len(timeline) != 2 {
		t.Fatalf("expected 2 timeline rows, got %#v", timeline)
	}
}
