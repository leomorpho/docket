package git

import (
	"os"
	"os/exec"
	"path/filepath"
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
	
	if err := MergeBranch(tmpDir, "feature-1"); err != nil {
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
	if err := MergeBranch(tmpDir, "no-such-branch"); err == nil {
		t.Error("expected error merging non-existent branch")
	}

	// 5. Delete non-existent
	if err := DeleteBranch(tmpDir, "no-such-branch"); err == nil {
		t.Error("expected error deleting non-existent branch")
	}
}
