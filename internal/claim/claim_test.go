package claim

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestClaim(t *testing.T) {
	repoRoot := t.TempDir()
	
	// Initialize git repo
	runGit := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
		_ = c.Run()
	}
	runGit("init")

	ticketID := "TKT-001"
	worktreePath := filepath.Join(repoRoot, "worktrees", "agent-1")
	agentID := "agent-1"

	// 1. Claim a ticket
	if err := Claim(repoRoot, ticketID, worktreePath, agentID); err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	// 2. Verify claim exists
	claim, err := GetClaim(repoRoot, ticketID)
	if err != nil {
		t.Fatalf("GetClaim failed: %v", err)
	}
	if claim == nil {
		t.Fatal("expected claim to exist")
	}
	if claim.AgentID != agentID {
		t.Errorf("expected agent %s, got %s", agentID, claim.AgentID)
	}

	// 3. Try to claim already claimed ticket (different agent)
	err = Claim(repoRoot, ticketID, "/other/path", "agent-2")
	if err == nil {
		t.Fatal("expected error when claiming already claimed ticket")
	}

	// 4. Release claim
	if err := Release(repoRoot, ticketID); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// 5. Verify claim is gone
	claim, err = GetClaim(repoRoot, ticketID)
	if err != nil {
		t.Fatalf("GetClaim failed after release: %v", err)
	}
	if claim != nil {
		t.Fatal("expected claim to be nil after release")
	}
}

func TestClaim_Idempotent(t *testing.T) {
	repoRoot := t.TempDir()
	c := exec.Command("git", "-C", repoRoot, "init")
	_ = c.Run()

	if err := Claim(repoRoot, "TKT-001", "/path", "agent"); err != nil {
		t.Fatal(err)
	}
	// Same agent/worktree should succeed
	if err := Claim(repoRoot, "TKT-001", "/path", "agent"); err != nil {
		t.Errorf("expected idempotent claim to succeed, got %v", err)
	}
}

func TestGetClaim_NoDir(t *testing.T) {
	repoRoot := t.TempDir()
	c := exec.Command("git", "-C", repoRoot, "init")
	_ = c.Run()

	// Should not fail if directory doesn't exist
	claim, err := GetClaim(repoRoot, "TKT-001")
	if err != nil {
		t.Errorf("GetClaim failed with no dir: %v", err)
	}
	if claim != nil {
		t.Error("expected nil claim")
	}
}

func TestClaim_CorruptJSON(t *testing.T) {
	repoRoot := t.TempDir()
	c := exec.Command("git", "-C", repoRoot, "init")
	_ = c.Run()

	claimsDir, _ := GetClaimsDir(repoRoot)
	os.MkdirAll(claimsDir, 0755)
	
	// Write corrupt JSON
	p := filepath.Join(claimsDir, "TKT-001.json")
	os.WriteFile(p, []byte("{invalid"), 0644)
	
	claim, err := GetClaim(repoRoot, "TKT-001")
	if err == nil {
		t.Error("expected error for corrupt JSON")
	}
	if claim != nil {
		t.Error("expected nil claim for corrupt JSON")
	}
}

func TestRelease_NotExists(t *testing.T) {
	repoRoot := t.TempDir()
	c := exec.Command("git", "-C", repoRoot, "init")
	_ = c.Run()

	// Should not fail if claim doesn't exist
	if err := Release(repoRoot, "TKT-999"); err != nil {
		t.Errorf("Release failed for non-existent claim: %v", err)
	}
}

func TestGetClaimsDir(t *testing.T) {
	repoRoot := t.TempDir()
	// Initialize git repo
	c := exec.Command("git", "-C", repoRoot, "init")
	_ = c.Run()

	expected := filepath.Join(repoRoot, ".git", "docket", "claims")
	actual, err := GetClaimsDir(repoRoot)
	if err != nil {
		t.Fatalf("GetClaimsDir failed: %v", err)
	}
	if actual != expected {
		t.Errorf("expected %s, got %s", expected, actual)
	}
}

func TestLocalClaimManager(t *testing.T) {
	repo := t.TempDir()
	// Initialize git repo
	c := exec.Command("git", "-C", repo, "init")
	_ = c.Run()

	m := NewLocalClaimManager(repo)
	ctx := context.Background()

	// 1. Claim
	err := m.Claim(ctx, "TKT-001", repo, "agent-1")
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	// 2. Get
	cl, err := m.GetClaim(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("GetClaim failed: %v", err)
	}
	if cl == nil || cl.AgentID != "agent-1" {
		t.Errorf("expected agent-1, got %v", cl)
	}

	// 3. Release
	err = m.Release(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	cl, _ = m.GetClaim(ctx, "TKT-001")
	if cl != nil {
		t.Error("expected nil claim after release")
	}
}
