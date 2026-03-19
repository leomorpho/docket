package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCreatesManagedArtifactsAndIsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	repo = tmpDir
	defer func() { repo = oldRepo }()
	format = "human"

	if err := os.MkdirAll(filepath.Join(tmpDir, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}

	// Reset global flags
	installSkill = false
	installCursor = false
	installVSCode = false

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"install"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	hookPath := preCommitHookPath(tmpDir)
	hookData, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("hook missing: %v", err)
	}
	if !strings.Contains(string(hookData), "__hook-lock-check") {
		t.Fatalf("pre-commit hook should run lock checks")
	}
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("hook stat failed: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("hook should be executable, mode=%v", info.Mode())
	}
	commitHookPath := commitMsgHookPath(tmpDir)
	commitHookData, err := os.ReadFile(commitHookPath)
	if err != nil {
		t.Fatalf("commit-msg hook missing: %v", err)
	}
	if !strings.Contains(string(commitHookData), "__hook-ac-check") || !strings.Contains(string(commitHookData), "Ticket: TKT-NNN") {
		t.Fatalf("commit-msg hook should enforce ticket trailer checks")
	}
	commitInfo, err := os.Stat(commitHookPath)
	if err != nil {
		t.Fatalf("commit-msg hook stat failed: %v", err)
	}
	if commitInfo.Mode()&0o111 == 0 {
		t.Fatalf("commit-msg hook should be executable, mode=%v", commitInfo.Mode())
	}

	manifestData, err := os.ReadFile(installManifestPath(tmpDir))
	if err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(manifestData, &m); err != nil {
		t.Fatalf("manifest invalid json: %v", err)
	}
	if m["docket_version"] == nil {
		t.Fatalf("manifest missing docket_version")
	}

	claudeData, err := os.ReadFile(claudePath(tmpDir))
	if err != nil {
		t.Fatalf("CLAUDE.md missing: %v", err)
	}
	if !strings.Contains(string(claudeData), docketBlockStart) || !strings.Contains(string(claudeData), docketBlockEnd) {
		t.Fatalf("CLAUDE.md missing managed markers")
	}

	before := string(claudeData)
	rootCmd.SetOut(new(bytes.Buffer))
	// Reset global flags
	installSkill = false
	installCursor = false
	installVSCode = false
	rootCmd.SetArgs([]string{"install"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	afterData, _ := os.ReadFile(claudePath(tmpDir))
	if string(afterData) != before {
		t.Fatalf("install should be idempotent for CLAUDE.md managed block")
	}
	gitignoreData, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("gitignore missing: %v", err)
	}
	if !strings.Contains(string(gitignoreData), ".docket/local/") {
		t.Fatalf("expected install to reconcile canonical local gitignore entry, got:\n%s", string(gitignoreData))
	}

	msgPath := filepath.Join(tmpDir, ".git", "COMMIT_EDITMSG")
	if err := os.WriteFile(msgPath, []byte("feat: test\n\nTicket: TKT-999\n"), 0o644); err != nil {
		t.Fatalf("write commit msg failed: %v", err)
	}
	ticketPath := filepath.Join(tmpDir, ".docket", "tickets")
	if err := os.MkdirAll(ticketPath, 0o755); err != nil {
		t.Fatalf("mkdir tickets failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ticketPath, "TKT-999.md"), []byte("state: done\n"), 0o644); err != nil {
		t.Fatalf("write done ticket failed: %v", err)
	}
	hookCmd := exec.Command(commitMsgHookPath(tmpDir), msgPath)
	hookCmd.Dir = tmpDir
	if err := hookCmd.Run(); err == nil {
		t.Fatalf("expected hook to block done-state referenced ticket")
	}
}

func TestInstallCommitMsgHookUsesCurrentCommitMessageFile(t *testing.T) {
	tmpDir := t.TempDir()
	if err := exec.Command("git", "init", "-q", tmpDir).Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("git config email failed: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.name", "tester").Run(); err != nil {
		t.Fatalf("git config name failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed failed: %v", err)
	}
	if out, err := exec.Command("git", "-C", tmpDir, "add", "seed.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add seed failed: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", tmpDir, "commit", "-m", "seed").CombinedOutput(); err != nil {
		t.Fatalf("git seed commit failed: %v\n%s", err, out)
	}
	if _, err := writeHook(tmpDir); err != nil {
		t.Fatalf("writeHook failed: %v", err)
	}

	logPath := filepath.Join(tmpDir, "hook.log")
	stubPath := filepath.Join(tmpDir, "docket-stub.sh")
	stub := "#!/bin/sh\nset -eu\nprintf '%s\\n' \"$*\" >> " + shellQuote(logPath) + "\n"
	if err := os.WriteFile(stubPath, []byte(stub), 0o755); err != nil {
		t.Fatalf("write docket stub failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, ".git", "COMMIT_EDITMSG"), []byte("old subject\n\nTicket: TKT-260\n"), 0o644); err != nil {
		t.Fatalf("write stale commit message failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "tracked.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write tracked file failed: %v", err)
	}
	worktreeDir := filepath.Join(t.TempDir(), "wt")
	if out, err := exec.Command("git", "-C", tmpDir, "worktree", "add", "-b", "docket/test-install-hook", worktreeDir).CombinedOutput(); err != nil {
		t.Fatalf("git worktree add failed: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(worktreeDir, "tracked.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write tracked file in worktree failed: %v", err)
	}
	addCmd := exec.Command("git", "-C", worktreeDir, "add", "tracked.txt")
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	commitCmd := exec.Command("git", "-C", worktreeDir, "commit", "-m", "new subject", "-m", "Ticket: TKT-253")
	commitCmd.Env = append(os.Environ(), "DOCKET_BIN="+stubPath)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read hook log failed: %v", err)
	}
	logText := string(logData)
	if !strings.Contains(logText, "__hook-ac-check TKT-253") {
		t.Fatalf("expected commit-msg hook to validate current ticket, got log:\n%s", logText)
	}
	if strings.Contains(logText, "__hook-ac-check TKT-260") {
		t.Fatalf("did not expect stale commit message ticket in hook log:\n%s", logText)
	}
}

func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
}

func TestUpgradeCheckAndApply(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	repo = tmpDir
	defer func() { repo = oldRepo }()
	format = "human"

	if err := os.MkdirAll(filepath.Join(tmpDir, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
	// Reset global flags
	installSkill = false
	installCursor = false
	installVSCode = false
	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"install"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	if err := os.WriteFile(preCommitHookPath(tmpDir), []byte("#!/bin/sh\necho stale\n"), 0o755); err != nil {
		t.Fatalf("write stale hook failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"upgrade", "--check"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected --check to fail for stale artifacts")
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"upgrade"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"upgrade", "--check"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected --check to pass after upgrade, got: %v", err)
	}

	cfgData, err := os.ReadFile(filepath.Join(tmpDir, ".docket", "config.yaml"))
	if err != nil {
		t.Fatalf("expected config.yaml to be managed by upgrade: %v", err)
	}
	if !strings.Contains(string(cfgData), "ticket_quality_min_ac:") {
		t.Fatalf("expected missing config key to be injected")
	}
	gitignoreData, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("expected upgrade to maintain gitignore: %v", err)
	}
	if !strings.Contains(string(gitignoreData), ".docket/local/") {
		t.Fatalf("expected upgrade to reconcile canonical local gitignore entry, got:\n%s", string(gitignoreData))
	}
}

func TestInstallHelpMentionsDocketHome(t *testing.T) {
	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"install", "--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("install --help failed: %v", err)
	}
	if !strings.Contains(out.String(), "DOCKET_HOME") {
		t.Fatalf("expected install help to mention DOCKET_HOME, got: %s", out.String())
	}
}
