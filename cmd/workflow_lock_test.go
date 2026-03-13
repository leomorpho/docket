package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
