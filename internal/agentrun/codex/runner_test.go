package codex

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/agentrun"
)

func TestRunnerStartsEphemeralCodexExecWithFreshSessionContract(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "codex.log")
	scriptPath := filepath.Join(tmp, "codex")
	script := "#!/bin/sh\n" +
		"printf 'ARGS:%s\\n' \"$*\" > \"$DOCKET_TEST_LOG\"\n" +
		"printf 'ENV_DOCKET_SESSION_ID:%s\\n' \"$DOCKET_SESSION_ID\" >> \"$DOCKET_TEST_LOG\"\n" +
		"printf 'ENV_DOCKET_TICKET_ID:%s\\n' \"$DOCKET_TICKET_ID\" >> \"$DOCKET_TEST_LOG\"\n" +
		"printf 'ENV_DOCKET_RUN_ROLE:%s\\n' \"$DOCKET_RUN_ROLE\" >> \"$DOCKET_TEST_LOG\"\n" +
		"cat - > \"$DOCKET_TEST_STDIN\"\n" +
		"printf 'RESULT status=done ticket=TKT-380 role=implementer commit=abc123 tests=passed\\n'\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	stdinPath := filepath.Join(tmp, "stdin.txt")
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKET_TEST_LOG", logPath)
	t.Setenv("DOCKET_TEST_STDIN", stdinPath)

	runner := NewRunner()
	spec := agentrun.RunSpec{
		TicketID:     "TKT-380",
		Role:         agentrun.RoleImplementer,
		RepoRoot:     "/repo",
		WorktreePath: "/repo/.worktrees/TKT-380",
		Branch:       "docket/TKT-380",
		Prompt:       "Use test-driven development.\nWork only ticket TKT-380 in this run.",
	}

	handle, record, err := runner.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer handle.Kill()

	body, err := io.ReadAll(handle.Stdout())
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if err := handle.Wait(); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}

	if err := record.Validate(); err != nil {
		t.Fatalf("record.Validate() error = %v", err)
	}
	if record.Adapter != "codex" {
		t.Fatalf("record.Adapter = %q, want codex", record.Adapter)
	}
	if strings.TrimSpace(record.SessionID) == "" {
		t.Fatalf("record.SessionID should be set: %#v", record)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(rawLog)
	for _, want := range []string{
		"-C /repo/.worktrees/TKT-380 exec --json --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox --ephemeral -",
		"ENV_DOCKET_TICKET_ID:TKT-380",
		"ENV_DOCKET_RUN_ROLE:implementer",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("log missing %q in %s", want, log)
		}
	}
	if !strings.Contains(log, "ENV_DOCKET_SESSION_ID:") {
		t.Fatalf("expected session id env in log: %s", log)
	}

	rawPrompt, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatalf("read stdin: %v", err)
	}
	if got := string(rawPrompt); got != spec.Prompt {
		t.Fatalf("prompt mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, spec.Prompt)
	}
	if !strings.Contains(string(body), "RESULT status=done") {
		t.Fatalf("stdout missing result line: %s", string(body))
	}
}

func TestSessionRunnerStartsPersistentCodexExecAndCanResume(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "codex.log")
	scriptPath := filepath.Join(tmp, "codex")
	script := "#!/bin/sh\n" +
		"printf 'ARGS:%s\\n' \"$*\" >> \"$DOCKET_TEST_LOG\"\n" +
		"cat - > \"$DOCKET_TEST_STDIN\"\n" +
		"printf 'RESULT status=done ticket=TKT-381 role=implementer commit=abc123 tests=passed\\n'\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	stdinPath := filepath.Join(tmp, "stdin.txt")
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKET_TEST_LOG", logPath)
	t.Setenv("DOCKET_TEST_STDIN", stdinPath)

	runner := NewSessionRunner()
	spec := agentrun.RunSpec{
		TicketID:     "TKT-381",
		Role:         agentrun.RoleImplementer,
		RepoRoot:     "/repo",
		WorktreePath: "/repo/.worktrees/TKT-381",
		Branch:       "docket/TKT-381",
		Prompt:       "continue",
	}

	handle, record, err := runner.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if _, err := io.ReadAll(handle.Stdout()); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if err := handle.Wait(); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if record.Adapter != "codex-session" {
		t.Fatalf("record.Adapter = %q, want codex-session", record.Adapter)
	}

	resumeHandle, resumeRecord, err := runner.Resume(context.Background(), "thread-123", spec)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if _, err := io.ReadAll(resumeHandle.Stdout()); err != nil {
		t.Fatalf("read resumed stdout: %v", err)
	}
	if err := resumeHandle.Wait(); err != nil {
		t.Fatalf("resume Wait() error = %v", err)
	}
	if resumeRecord.SessionID != "thread-123" {
		t.Fatalf("resumeRecord.SessionID = %q, want thread-123", resumeRecord.SessionID)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(rawLog)
	if strings.Contains(log, "--ephemeral") {
		t.Fatalf("session runner should not use --ephemeral: %s", log)
	}
	if !strings.Contains(log, "-C /repo/.worktrees/TKT-381 exec --json --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox -") {
		t.Fatalf("session start args missing in log: %s", log)
	}
	if !strings.Contains(log, "-C /repo/.worktrees/TKT-381 exec resume --json --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox thread-123 -") {
		t.Fatalf("session resume args missing in log: %s", log)
	}
}
