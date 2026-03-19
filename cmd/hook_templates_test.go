package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLefthookTemplatesRunTicketChecksInCommitMsg(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))

	tests := []string{
		filepath.Join(repoRoot, "lefthook.yml"),
		filepath.Join(repoRoot, "templates", "lefthook.yml"),
	}

	for _, path := range tests {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			content := string(data)
			if strings.Contains(content, `git log -1 --format="%B"`) {
				t.Fatalf("%s should not inspect the previous commit message", path)
			}
			if !strings.Contains(content, "commit-msg:") {
				t.Fatalf("%s should define a commit-msg hook for Ticket trailer checks", path)
			}
			if !strings.Contains(content, `MSG_FILE="{1}"`) {
				t.Fatalf("%s should read the commit message path from the commit-msg hook argument", path)
			}
			if !strings.Contains(content, `grep -Eo 'Ticket:[[:space:]]*TKT-[0-9]+' "$MSG_FILE"`) {
				t.Fatalf("%s should extract Ticket trailers from MSG_FILE", path)
			}
			if !strings.Contains(content, `if [ -d "$REPO_ROOT/.git" ]; then`) {
				t.Fatalf("%s should reject Ticket-linked commits from the primary checkout", path)
			}
			preCommitSection := strings.Split(content, "commit-msg:")[0]
			if strings.Contains(preCommitSection, "COMMIT_EDITMSG") {
				t.Fatalf("%s pre-commit hook should not read COMMIT_EDITMSG directly", path)
			}
		})
	}
}
