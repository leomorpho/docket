package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
)

func TestCreateCmd(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	// 0. docket init first
	s := local.New(tmpDir)
	cfg := ticket.DefaultConfig()
	ticket.SaveConfig(tmpDir, cfg)

	// 1. Create first ticket
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"create", "--title", "First Ticket", "--priority", "1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if !strings.Contains(b.String(), "Created TKT-001") {
		t.Errorf("expected TKT-001 created, got: %s", b.String())
	}

	// 2. Verify file exists and has content
	path := filepath.Join(tmpDir, ".docket", "tickets", "TKT-001.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("TKT-001.md not created")
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "title: First Ticket") {
		// Note: render() might use a slightly different YAML format, but let's check title in body too
		if !strings.Contains(string(data), "# TKT-001: First Ticket") {
			t.Errorf("ticket file does not contain title: %s", string(data))
		}
	}

	// 3. Create second ticket with DOCKET_ACTOR
	os.Setenv("DOCKET_ACTOR", "agent:test-model")
	defer os.Unsetenv("DOCKET_ACTOR")
	
	b.Reset()
	rootCmd.SetArgs([]string{"create", "--title", "Second Ticket", "--labels", "feat,llm"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("second create failed: %v", err)
	}
	if !strings.Contains(b.String(), "Created TKT-002") {
		t.Errorf("expected TKT-002 created, got: %s", b.String())
	}

	t2, _ := s.GetTicket(context.Background(), "TKT-002")
	if t2.CreatedBy != "agent:test-model" {
		t.Errorf("CreatedBy mismatch: %s", t2.CreatedBy)
	}
	if len(t2.Labels) != 2 || t2.Labels[0] != "feat" {
		t.Errorf("Labels mismatch: %v", t2.Labels)
	}

	// 4. JSON output
	format = "json"
	b.Reset()
	rootCmd.SetArgs([]string{"create", "--title", "JSON Ticket"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("JSON create failed: %v", err)
	}
	var res map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if res["id"] != "TKT-003" {
		t.Errorf("expected TKT-003 in JSON, got: %v", res["id"])
	}
}

func TestCreateCmd_ValidationError(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	
	// Reset flags because they are global
	title = ""
	
	rootCmd.SetArgs([]string{"create"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing title, got nil")
	}
	if !strings.Contains(err.Error(), "--title is required") {
		t.Errorf("expected '--title is required' error, got: %v", err)
	}

	// Test invalid state
	rootCmd.SetArgs([]string{"create", "--title", "T", "--state", "invalid"})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid state, got nil")
	}
}
