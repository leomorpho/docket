package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestServeMCP_ContentFile(t *testing.T) {
	repo := t.TempDir()
	if err := ticket.SaveConfig(repo, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	tempFile := filepath.Join(repo, "large_content.txt")
	content := "This is a very large description from a file."
	if err := os.WriteFile(tempFile, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	in := strings.NewReader(`{"id":1,"action":"create","args":{"title":"From file","content_file":"` + tempFile + `"}}
`)
	var out bytes.Buffer
	if err := ServeMCP(in, &out, repo); err != nil {
		t.Fatalf("ServeMCP failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var r1 map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &r1); err != nil {
		t.Fatal(err)
	}

	if r1["ok"] != true {
		t.Fatalf("create response not ok: %v", r1)
	}

	result := r1["result"].(map[string]interface{})
	id := result["id"].(string)

	// Now show the ticket to verify content
	in2 := strings.NewReader(`{"id":2,"action":"show","args":{"id":"` + id + `"}}
`)
	var out2 bytes.Buffer
	if err := ServeMCP(in2, &out2, repo); err != nil {
		t.Fatalf("ServeMCP show failed: %v", err)
	}

	var r2 map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out2.String())), &r2); err != nil {
		t.Fatal(err)
	}

	if r2["ok"] != true {
		t.Fatalf("show response not ok: %v", r2)
	}

	ticketResult := r2["result"].(map[string]interface{})
	if ticketResult["description"] != content {
		t.Fatalf("expected description %q, got %q", content, ticketResult["description"])
	}

	// Test update with content_file
	updateFile := filepath.Join(repo, "update_content.txt")
	updateContent := "Updated content from file."
	os.WriteFile(updateFile, []byte(updateContent), 0644)

	in3 := strings.NewReader(`{"id":3,"action":"update","args":{"id":"` + id + `","content_file":"` + updateFile + `"}}
`)
	var out3 bytes.Buffer
	ServeMCP(in3, &out3, repo)

	var r3 map[string]interface{}
	json.Unmarshal([]byte(strings.TrimSpace(out3.String())), &r3)
	if r3["ok"] != true {
		t.Fatalf("update response not ok: %v", r3)
	}

	// Show again
	in4 := strings.NewReader(`{"id":4,"action":"show","args":{"id":"` + id + `"}}
`)
	var out4 bytes.Buffer
	ServeMCP(in4, &out4, repo)
	var r4 map[string]interface{}
	json.Unmarshal([]byte(strings.TrimSpace(out4.String())), &r4)
	ticketResult2 := r4["result"].(map[string]interface{})
	if ticketResult2["description"] != updateContent {
		t.Fatalf("expected description %q, got %q", updateContent, ticketResult2["description"])
	}

	// Test security: attempt to read file outside repo and not in /tmp
	// (Note: /home/developer is NOT in repo or /tmp)
	badFile := "/etc/passwd"
	in5 := strings.NewReader(`{"id":5,"action":"create","args":{"title":"Bad","content_file":"` + badFile + `"}}
`)
	var out5 bytes.Buffer
	ServeMCP(in5, &out5, repo)
	var r5 map[string]interface{}
	json.Unmarshal([]byte(strings.TrimSpace(out5.String())), &r5)
	
	// Since handleCreate ignores content_file if getContent returns false, it will still succeed but with empty desc.
	// Wait, handleCreate: desc, _ := getContent(...)
	// So if getContent returns false, desc is empty.
	
	result5 := r5["result"].(map[string]interface{})
	id5 := result5["id"].(string)

	in6 := strings.NewReader(`{"id":6,"action":"show","args":{"id":"` + id5 + `"}}
`)
	var out6 bytes.Buffer
	ServeMCP(in6, &out6, repo)
	var r6 map[string]interface{}
	json.Unmarshal([]byte(strings.TrimSpace(out6.String())), &r6)
	ticketResult5 := r6["result"].(map[string]interface{})
	desc5, _ := ticketResult5["description"].(string)
	if desc5 != "" {
		t.Logf("DEBUG: desc5=%q", desc5)
		t.Fatalf("security violation: read %s", badFile)
	}
}
