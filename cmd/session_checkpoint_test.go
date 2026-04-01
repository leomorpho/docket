package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestACCompleteWritesCheckpointAndSessionResume(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"
	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-501", Seq: 501, Title: "Checkpoint", State: "draft", Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}},
	})

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"ac", "complete", "TKT-501", "--step", "1", "--evidence", "done"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ac complete failed: %v", err)
	}
	paths, err := listCheckpointPaths(tmp, "TKT-501")
	if err != nil || len(paths) == 0 {
		t.Fatalf("expected checkpoint after ac complete, err=%v paths=%v", err, paths)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"session", "resume", "TKT-501"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("session resume failed: %v", err)
	}
	if !strings.Contains(out.String(), "RESUME_CONTEXT") || !strings.Contains(out.String(), "ac=") {
		t.Fatalf("unexpected resume output: %s", out.String())
	}
}

func TestSessionCompressCheckpointAndListIncludesCheckpoints(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"
	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-502", Seq: 502, Title: "Compress", State: "draft", Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}},
	})
	// attach session file
	sessionSource := filepath.Join(tmp, "session.log")
	_ = os.WriteFile(sessionSource, []byte("hello"), 0o644)
	if _, err := s.AttachSession(context.Background(), "TKT-502", sessionSource); err != nil {
		t.Fatalf("attach session failed: %v", err)
	}
	summaryPath := filepath.Join(tmp, "summary.md")
	_ = os.WriteFile(summaryPath, []byte("## Handoff\n\nSummary text"), 0o644)

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"session", "compress", "TKT-502", "--summary-file", summaryPath, "--checkpoint"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("session compress failed: %v", err)
	}

	paths, err := listCheckpointPaths(tmp, "TKT-502")
	if err != nil || len(paths) == 0 {
		t.Fatalf("expected checkpoint after session compress --checkpoint, err=%v paths=%v", err, paths)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"session", "list", "TKT-502"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("session list failed: %v", err)
	}
	if !strings.Contains(out.String(), "Checkpoints for TKT-502") {
		t.Fatalf("expected checkpoint listing, got: %s", out.String())
	}
}

func TestSessionResume_RejectsAgentOutsideBoundWorktree(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"

	runGitSession(t, tmp, "init")
	runGitSession(t, tmp, "config", "user.email", "test@example.com")
	runGitSession(t, tmp, "config", "user.name", "Test User")

	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-503", Seq: 503, Title: "Checkpoint", State: "draft", Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}},
	})

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"ac", "complete", "TKT-503", "--step", "1", "--evidence", "done"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ac complete failed: %v", err)
	}

	worktreePath := filepath.Join(tmp, "wt", "TKT-503")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("creating worktree path failed: %v", err)
	}
	if err := claim.Claim(tmp, "TKT-503", worktreePath, "agent:test"); err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	t.Setenv("DOCKET_AGENT_ID", "test")
	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"session", "resume", "TKT-503"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "must run inside bound worktree") {
		t.Fatalf("expected bound worktree rejection, got: %v", err)
	}
}

func TestSessionResume_RejectsAgentWithoutRunManifest(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"

	runGitSession(t, tmp, "init")
	runGitSession(t, tmp, "config", "user.email", "test@example.com")
	runGitSession(t, tmp, "config", "user.name", "Test User")

	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-505", Seq: 505, Title: "Resume manifest gate", State: "draft", Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}},
	})

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"ac", "complete", "TKT-505", "--step", "1", "--evidence", "done"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ac complete failed: %v", err)
	}

	worktreePath := filepath.Join(tmp, "wt", "TKT-505")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("creating worktree path failed: %v", err)
	}
	resolvedWorktreePath, err := filepath.EvalSymlinks(worktreePath)
	if err != nil {
		t.Fatalf("eval symlinks failed: %v", err)
	}
	if err := claim.Claim(tmp, "TKT-505", resolvedWorktreePath, "agent:test"); err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(resolvedWorktreePath); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	t.Setenv("DOCKET_AGENT_ID", "test")
	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"session", "resume", "TKT-505"})
	err = rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "requires run manifest") {
		t.Fatalf("expected run manifest rejection, got: %v", err)
	}
}

func TestBuildCheckpointIncludesStructuredResumeFields(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"

	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:            "TKT-504",
		Seq:           504,
		Title:         "Structured checkpoint",
		State:         "running",
		Priority:      1,
		BlockedBy:     []string{"TKT-099"},
		LinkedCommits: []string{"abc123"},
		CreatedAt:     now,
		UpdatedAt:     now,
		CreatedBy:     "me",
		Description:   "desc",
		AC: []ticket.AcceptanceCriterion{
			{Description: "Done step", Done: true},
			{Description: "Pending step", Done: false},
		},
	})

	cp := buildCheckpoint(tmp, "TKT-504", "summary")
	if len(cp.LinkedCommits) != 1 || cp.LinkedCommits[0] != "abc123" {
		t.Fatalf("expected linked commits in checkpoint, got %#v", cp.LinkedCommits)
	}
	if len(cp.Blockers) != 1 || cp.Blockers[0] != "TKT-099" {
		t.Fatalf("expected blockers in checkpoint, got %#v", cp.Blockers)
	}
	if len(cp.NextSteps) != 1 || !strings.Contains(cp.NextSteps[0], "Pending step") {
		t.Fatalf("expected next steps in checkpoint, got %#v", cp.NextSteps)
	}
	if cp.TicketState != "running" {
		t.Fatalf("expected ticket state in checkpoint, got %q", cp.TicketState)
	}
}

func TestSessionCompressResumeContinuityPacket(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"

	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-506",
		Seq:         506,
		Title:       "Continuity packet",
		State:       "running",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "desc",
		AC: []ticket.AcceptanceCriterion{
			{Description: "Done step", Done: true},
			{Description: "Pending step", Done: false},
		},
	})

	sessionSource := filepath.Join(tmp, "session-506.log")
	_ = os.WriteFile(sessionSource, []byte("session context"), 0o644)
	if _, err := s.AttachSession(context.Background(), "TKT-506", sessionSource); err != nil {
		t.Fatalf("attach session failed: %v", err)
	}
	summaryPath := filepath.Join(tmp, "summary-506.md")
	_ = os.WriteFile(summaryPath, []byte("## Handoff\n\nNext up: finish pending AC"), 0o644)

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"session", "compress", "TKT-506", "--summary-file", summaryPath, "--checkpoint"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("session compress failed: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"session", "resume", "TKT-506"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("session resume failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "state=running") {
		t.Fatalf("expected resume packet to include ticket state, got: %s", got)
	}
	if !strings.Contains(got, "next_steps=[Pending step]") {
		t.Fatalf("expected resume packet to include next steps, got: %s", got)
	}
	if !strings.Contains(got, "summary=Next up: finish pending AC") {
		t.Fatalf("expected resume packet to include handoff summary, got: %s", got)
	}
	if !strings.Contains(got, "last_comments=[Session compressed. Handoff updated.]") {
		t.Fatalf("expected resume packet to include recent execution context comment, got: %s", got)
	}
}

func TestSessionResumeFallsBackToManagedRunBrief(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"

	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:            "TKT-507",
		Seq:           507,
		Title:         "Brief fallback",
		State:         "running",
		Priority:      1,
		LinkedCommits: []string{"abc123"},
		CreatedAt:     now,
		UpdatedAt:     now,
		CreatedBy:     "me",
		Description:   "desc",
		AC: []ticket.AcceptanceCriterion{
			{Description: "Done step", Done: true},
			{Description: "Pending repair", Done: false},
		},
	})
	if err := runruntime.New(tmp).WriteBrief(runruntime.RunBrief{
		TicketID:         "TKT-507",
		Outcome:          "failed",
		Summary:          "Managed run failed validation before closeout.",
		CommitSHA:        "def456",
		FilesTouched:     []string{"feature.txt", "README.md"},
		Tests:            "go test ./...",
		ValidationErrors: []string{"feature.txt missing"},
		ResumeNext:       "Repair feature.txt and rerun the managed ticket.",
		UpdatedAt:        now.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("WriteBrief() failed: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"session", "resume", "TKT-507"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("session resume failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"RESUME_CONTEXT",
		"ticket=TKT-507",
		"state=running",
		"linked_commits=[abc123, def456]",
		"changed_files=[feature.txt, README.md]",
		"next_steps=[Pending repair | Repair feature.txt and rerun the managed ticket.]",
		"summary=Managed run failed validation before closeout.",
		"last_comments=[Validation: go test ./... | Validation errors: feature.txt missing]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got: %s", want, got)
		}
	}
}
