package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMerge(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo
	runGit := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", tmpDir}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")

	// Create initial commit
	os.WriteFile(filepath.Join(tmpDir, "README"), []byte("root"), 0644)
	runGit("add", "README")
	runGit("commit", "-m", "initial")

	// 1. Get default branch
	def, err := GetDefaultBranch(tmpDir)
	if err != nil {
		t.Fatalf("GetDefaultBranch failed: %v", err)
	}
	if def != "main" && def != "master" {
		t.Errorf("unexpected default branch: %s", def)
	}

	// 2. Create and merge branch
	runGit("checkout", "-b", "feature-1")
	os.WriteFile(filepath.Join(tmpDir, "feature.txt"), []byte("feat"), 0644)
	runGit("add", "feature.txt")
	runGit("commit", "-m", "feature")

	runGit("checkout", def)

	if err := MergeBranch(tmpDir, "feature-1", ""); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "feature.txt")); err != nil {
		t.Fatal("expected merged file to exist")
	}

	// 3. Delete branch
	if err := DeleteBranch(tmpDir, "feature-1"); err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	// 4. Merge non-existent
	if err := MergeBranch(tmpDir, "no-such-branch", ""); err == nil {
		t.Error("expected error merging non-existent branch")
	}

	// 5. Delete non-existent
	if err := DeleteBranch(tmpDir, "no-such-branch"); err == nil {
		t.Error("expected error deleting non-existent branch")
	}
}

func TestMergeBranch_AutostashesUnrelatedDirtyTrackedChanges(t *testing.T) {
	tmpDir := t.TempDir()

	runGit := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", tmpDir}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")

	dirtyPath := filepath.Join(tmpDir, "docs.txt")
	if err := os.WriteFile(dirtyPath, []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write docs.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "README"), []byte("root\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "initial")

	def, err := GetDefaultBranch(tmpDir)
	if err != nil {
		t.Fatalf("GetDefaultBranch failed: %v", err)
	}

	runGit("checkout", "-b", "feature-1")
	if err := os.WriteFile(filepath.Join(tmpDir, "feature.txt"), []byte("feat\n"), 0o644); err != nil {
		t.Fatalf("write feature.txt: %v", err)
	}
	runGit("add", "feature.txt")
	runGit("commit", "-m", "feature")
	runGit("checkout", def)

	if err := os.WriteFile(dirtyPath, []byte("base\nlocal dirty change\n"), 0o644); err != nil {
		t.Fatalf("dirty docs.txt: %v", err)
	}

	if err := MergeBranch(tmpDir, "feature-1", ""); err != nil {
		t.Fatalf("MergeBranch with dirty tracked file failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "feature.txt")); err != nil {
		t.Fatalf("expected merged file to exist: %v", err)
	}
	data, err := os.ReadFile(dirtyPath)
	if err != nil {
		t.Fatalf("read dirty file after merge: %v", err)
	}
	if !strings.Contains(string(data), "local dirty change") {
		t.Fatalf("expected dirty tracked changes to survive merge, got %q", string(data))
	}
	statusCmd := exec.Command("git", "-C", tmpDir, "status", "--porcelain")
	out, err := statusCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status failed: %v (%s)", err, string(out))
	}
	if got := strings.TrimSpace(string(out)); got != " M docs.txt" && got != "M docs.txt" {
		t.Fatalf("expected only docs.txt to remain dirty after merge, got %q", got)
	}
}

func TestMergeBranch_UsesExplicitMessage(t *testing.T) {
	tmpDir := t.TempDir()

	runGit := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", tmpDir}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")

	if err := os.WriteFile(filepath.Join(tmpDir, "README"), []byte("root\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit("add", "README")
	runGit("commit", "-m", "initial")

	def, err := GetDefaultBranch(tmpDir)
	if err != nil {
		t.Fatalf("GetDefaultBranch failed: %v", err)
	}

	runGit("checkout", "-b", "feature-1")
	if err := os.WriteFile(filepath.Join(tmpDir, "feature.txt"), []byte("feat\n"), 0o644); err != nil {
		t.Fatalf("write feature.txt: %v", err)
	}
	runGit("add", "feature.txt")
	runGit("commit", "-m", "feature")
	runGit("checkout", def)

	message := "docket: close out TKT-001\n\nTicket: TKT-001"
	if err := MergeBranch(tmpDir, "feature-1", message); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}

	c := exec.Command("git", "-C", tmpDir, "log", "-1", "--format=%B")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git log failed: %v (%s)", err, string(out))
	}
	if got := strings.TrimSpace(string(out)); got != message {
		t.Fatalf("merge commit message = %q, want %q", got, message)
	}
}

func TestMergeBranch_BypassesPrimaryCheckoutCommitHooks(t *testing.T) {
	tmpDir := t.TempDir()

	runGit := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", tmpDir}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")

	if err := os.WriteFile(filepath.Join(tmpDir, "README"), []byte("root\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit("add", "README")
	runGit("commit", "-m", "initial")

	hook := "#!/bin/sh\nif grep -q 'Ticket: TKT-' \"$1\"; then\n  echo 'hook rejected ticket trailer in primary checkout' >&2\n  exit 1\nfi\n"
	hookPath := filepath.Join(tmpDir, ".git", "hooks", "commit-msg")
	if err := os.WriteFile(hookPath, []byte(hook), 0o755); err != nil {
		t.Fatalf("write commit-msg hook: %v", err)
	}

	def, err := GetDefaultBranch(tmpDir)
	if err != nil {
		t.Fatalf("GetDefaultBranch failed: %v", err)
	}

	runGit("checkout", "-b", "feature-1")
	if err := os.WriteFile(filepath.Join(tmpDir, "feature.txt"), []byte("feat\n"), 0o644); err != nil {
		t.Fatalf("write feature.txt: %v", err)
	}
	runGit("add", "feature.txt")
	runGit("commit", "-m", "feature")
	runGit("checkout", def)

	message := "docket: close out TKT-001\n\nTicket: TKT-001"
	if err := MergeBranch(tmpDir, "feature-1", message); err != nil {
		t.Fatalf("MergeBranch failed with primary-checkout hook installed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "feature.txt")); err != nil {
		t.Fatalf("expected merged file to exist: %v", err)
	}
}
