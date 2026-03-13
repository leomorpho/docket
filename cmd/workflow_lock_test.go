package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/security"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/workflow"
)

func TestWorkflowLockGenerateAndValidate(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	t.Setenv("DOCKET_KEYSTORE_PASSWORD", "pw-1")
	docketHome = ""
	repo = tmpRepo
	format = "human"

	if err := os.MkdirAll(filepath.Join(tmpRepo, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir docket failed: %v", err)
	}
	proposalPath := filepath.Join(tmpRepo, workflow.DefaultWorkflowPolicy)
	if err := os.WriteFile(proposalPath, []byte(`{"states":{"todo":["in-progress"],"in-progress":["done"]},"prompt_pack":"v1"}`), 0o644); err != nil {
		t.Fatalf("write proposal failed: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)

	rootCmd.SetArgs([]string{"secure", "unlock", "--password", "pw-1", "--ttl", "5m"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("secure unlock failed: %v", err)
	}

	out.Reset()
	rootCmd.SetArgs([]string{"workflow", "lock", "generate", "--proposal", workflow.DefaultWorkflowPolicy, "--signer-id", "local-signer", "--ticket", "TKT-183", "--yes"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workflow lock generate failed: %v", err)
	}
	if !strings.Contains(out.String(), "Generated") {
		t.Fatalf("expected generated output, got: %s", out.String())
	}

	out.Reset()
	rootCmd.SetArgs([]string{"workflow", "lock", "validate"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workflow lock validate failed: %v", err)
	}
}

func TestWorkflowLockValidateRejectsStale(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	t.Setenv("DOCKET_KEYSTORE_PASSWORD", "pw-1")
	docketHome = ""
	repo = tmpRepo
	format = "human"

	if err := os.MkdirAll(filepath.Join(tmpRepo, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir docket failed: %v", err)
	}
	proposalPath := filepath.Join(tmpRepo, workflow.DefaultWorkflowPolicy)
	if err := os.WriteFile(proposalPath, []byte(`{"states":{"todo":["done"]}}`), 0o644); err != nil {
		t.Fatalf("write proposal failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"secure", "unlock", "--password", "pw-1", "--ttl", "5m"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("secure unlock failed: %v", err)
	}

	rootCmd.SetArgs([]string{"workflow", "lock", "generate", "--proposal", workflow.DefaultWorkflowPolicy, "--signer-id", "local-signer", "--ticket", "TKT-183", "--yes"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workflow lock generate failed: %v", err)
	}

	if err := os.WriteFile(proposalPath, []byte(`{"states":{"todo":["in-progress"]}}`), 0o644); err != nil {
		t.Fatalf("mutate proposal failed: %v", err)
	}

	rootCmd.SetArgs([]string{"workflow", "lock", "validate"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected stale lock error, got: %v", err)
	}
}

func TestWorkflowLockActivateRequiresSecureMode(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	t.Setenv("DOCKET_KEYSTORE_PASSWORD", "pw-1")
	docketHome = ""
	repo = tmpRepo
	format = "human"

	if err := os.MkdirAll(filepath.Join(tmpRepo, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir docket failed: %v", err)
	}
	proposalPath := filepath.Join(tmpRepo, workflow.DefaultWorkflowPolicy)
	if err := os.WriteFile(proposalPath, []byte(`{"states":{"todo":["done"]}}`), 0o644); err != nil {
		t.Fatalf("write proposal failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"secure", "unlock", "--password", "pw-1", "--ttl", "5m"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("secure unlock failed: %v", err)
	}
	rootCmd.SetArgs([]string{"workflow", "lock", "generate", "--signer-id", "local-signer", "--ticket", "TKT-184", "--yes"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workflow lock generate failed: %v", err)
	}
	rootCmd.SetArgs([]string{"secure", "lock"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("secure lock failed: %v", err)
	}

	rootCmd.SetArgs([]string{"workflow", "lock", "activate", "--ticket", "TKT-184", "--yes"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "secure mode is inactive") {
		t.Fatalf("expected secure mode inactive error, got: %v", err)
	}
}

func TestStartRunPinnedToActivatedWorkflowHash(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	t.Setenv("DOCKET_KEYSTORE_PASSWORD", "pw-1")
	docketHome = ""
	repo = tmpRepo
	format = "human"

	if err := os.MkdirAll(filepath.Join(tmpRepo, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir docket failed: %v", err)
	}
	cfg := ticket.DefaultConfig()
	backlog := cfg.States["backlog"]
	backlog.Next = append(backlog.Next, "in-progress")
	cfg.States["backlog"] = backlog
	if err := ticket.SaveConfig(tmpRepo, cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	proposalPath := filepath.Join(tmpRepo, workflow.DefaultWorkflowPolicy)
	if err := os.WriteFile(proposalPath, []byte(`{"states":{"backlog":["in-progress"],"in-progress":["done"]}}`), 0o644); err != nil {
		t.Fatalf("write proposal failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"secure", "unlock", "--password", "pw-1", "--ttl", "5m"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("secure unlock failed: %v", err)
	}
	rootCmd.SetArgs([]string{"workflow", "lock", "generate", "--signer-id", "local-signer", "--ticket", "TKT-184", "--yes"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workflow lock generate failed: %v", err)
	}
	rootCmd.SetArgs([]string{"workflow", "lock", "activate", "--ticket", "TKT-184", "--yes"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workflow lock activate failed: %v", err)
	}

	ns := security.NewRepoNamespaceStore(tmpHome)
	pinnedHash, ok, err := ns.GetActiveWorkflowHash(tmpRepo)
	if err != nil || !ok {
		t.Fatalf("failed to read active workflow hash: ok=%v err=%v", ok, err)
	}

	// Proposal changes after activation should not repin existing activation automatically.
	if err := os.WriteFile(proposalPath, []byte(`{"states":{"backlog":["in-progress"],"in-progress":["in-review"]}}`), 0o644); err != nil {
		t.Fatalf("mutate proposal failed: %v", err)
	}

	s := local.New(tmpRepo)
	now := time.Now().UTC()
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-901",
		Seq:         901,
		Title:       "Pinned Run",
		State:       ticket.State("backlog"),
		Priority:    1,
		Description: "Test run pinning ticket with enough detail for workflow.",
		AC:          []ticket.AcceptanceCriterion{{Description: "Do thing", Done: false}},
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "test",
	}); err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}

	rootCmd.SetArgs([]string{"start"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	run, ok, err := ns.GetRunManifest(tmpRepo, "TKT-901")
	if err != nil || !ok {
		t.Fatalf("expected run manifest, ok=%v err=%v", ok, err)
	}
	if run.WorkflowHash != pinnedHash {
		t.Fatalf("run should be pinned to activated hash %s, got %s", pinnedHash, run.WorkflowHash)
	}
}
