package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	docketgit "github.com/leomorpho/docket/internal/git"
	"github.com/leomorpho/docket/internal/security"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

type fakeRepoHarness struct {
	t    *testing.T
	repo string
	home string
}

func newFakeRepoHarness(t *testing.T) *fakeRepoHarness {
	return newFakeRepoHarnessForAdapter(t, "codex")
}

func newFakeRepoHarnessForAdapter(t *testing.T, adapterID string) *fakeRepoHarness {
	t.Helper()

	repoRoot := t.TempDir()
	home := filepath.Join(t.TempDir(), "docket-home")
	userHome := filepath.Join(t.TempDir(), "home")
	t.Setenv("DOCKET_HOME", home)
	t.Setenv("HOME", userHome)
	t.Setenv("DOCKET_AGENT_ID", "harness-agent")
	docketHome = ""
	repo = repoRoot
	format = "human"

	runGitSession(t, repoRoot, "init")
	runGitSession(t, repoRoot, "config", "user.email", "test@example.com")
	runGitSession(t, repoRoot, "config", "user.name", "Harness User")
	if err := os.WriteFile(filepath.Join(repoRoot, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "doombox.json"), []byte(`{"mcp":"docket"}`), 0o644); err != nil {
		t.Fatalf("write doombox.json failed: %v", err)
	}
	switch adapterID {
	case "codex":
		if err := os.WriteFile(filepath.Join(repoRoot, "AGENTS.md"), []byte("codex marker"), 0o644); err != nil {
			t.Fatalf("write AGENTS.md failed: %v", err)
		}
	case "claude-code":
		if err := os.WriteFile(filepath.Join(repoRoot, "CLAUDE.md"), []byte("claude marker"), 0o644); err != nil {
			t.Fatalf("write CLAUDE.md failed: %v", err)
		}
	case "gemini":
		if err := os.WriteFile(filepath.Join(repoRoot, "GEMINI.md"), []byte("gemini marker"), 0o644); err != nil {
			t.Fatalf("write GEMINI.md failed: %v", err)
		}
		skillPath := filepath.Join(userHome, ".gemini", "skills", "docket", "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
			t.Fatalf("mkdir gemini skill path failed: %v", err)
		}
		if err := os.WriteFile(skillPath, []byte("# skill"), 0o644); err != nil {
			t.Fatalf("write gemini SKILL.md failed: %v", err)
		}
	default:
		t.Fatalf("unsupported adapter fixture %q", adapterID)
	}
	runGitSession(t, repoRoot, "add", ".")
	runGitSession(t, repoRoot, "commit", "-m", "chore: seed harness")

	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	return &fakeRepoHarness{t: t, repo: repoRoot, home: home}
}

func (h *fakeRepoHarness) seedTicket(id string, seq int, state ticket.State, ac []ticket.AcceptanceCriterion) {
	h.t.Helper()
	if wt, err := docketgit.GetAgentWorktreeDir(id); err == nil {
		_ = os.RemoveAll(wt)
	}

	s := local.New(h.repo)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          id,
		Seq:         seq,
		Title:       "Harness " + id,
		State:       state,
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:harness-agent",
		Description: "harness ticket",
		AC:          ac,
	}); err != nil {
		h.t.Fatalf("seed ticket %s failed: %v", id, err)
	}
}

func (h *fakeRepoHarness) run(args ...string) (string, error) {
	h.t.Helper()
	repo = h.repo
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return out.String(), err
}

func (h *fakeRepoHarness) writeFixture(name string, data []byte) string {
	h.t.Helper()
	path := filepath.Join(h.repo, ".docket", "test-fixtures", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		h.t.Fatalf("mkdir fixtures failed: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		h.t.Fatalf("write fixture %s failed: %v", path, err)
	}
	return path
}

func TestFakeRepoHarnessSetupAndSeedHelpers(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-901", 901, ticket.State("todo"), []ticket.AcceptanceCriterion{{Description: "ac"}})

	out, err := h.run("show", "TKT-901", "--format", "json")
	if err != nil {
		t.Fatalf("show failed: %v\n%s", err, out)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal show json failed: %v\n%s", err, out)
	}
	if payload["id"] != "TKT-901" {
		t.Fatalf("expected seeded ticket id, got %v", payload["id"])
	}
}

func TestFakeRepoHarnessCommandWrapper(t *testing.T) {
	h := newFakeRepoHarness(t)
	out, err := h.run("doctor", "--format", "json")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "\"checks\"") {
		t.Fatalf("expected doctor json output, got: %s", out)
	}
}

func TestFakeRepoHarnessHappyPathIntegration(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-902", 902, ticket.State("todo"), []ticket.AcceptanceCriterion{{Description: "ac"}})

	if out, err := h.run("bootstrap", "--adapter", "codex"); err != nil {
		t.Fatalf("bootstrap failed: %v\n%s", err, out)
	}

	startOut, err := h.run("start", "--format", "json")
	if err != nil {
		t.Fatalf("start failed: %v\n%s", err, startOut)
	}
	var startPayload map[string]any
	if err := json.Unmarshal([]byte(startOut), &startPayload); err != nil {
		t.Fatalf("unmarshal start json failed: %v\n%s", err, startOut)
	}
	ticketPayload := startPayload["ticket"].(map[string]any)
	if ticketPayload["id"] != "TKT-902" {
		t.Fatalf("expected started ticket TKT-902, got %v", ticketPayload["id"])
	}

	doctorOut, err := h.run("doctor", "--format", "json")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, doctorOut)
	}
	var doctorPayload map[string]any
	if err := json.Unmarshal([]byte(doctorOut), &doctorPayload); err != nil {
		t.Fatalf("unmarshal doctor json failed: %v\n%s", err, doctorOut)
	}

	ns := security.NewRepoNamespaceStore(h.home)
	run, ok, err := ns.GetRunManifest(h.repo, "TKT-902")
	if err != nil || !ok {
		t.Fatalf("expected run manifest for TKT-902, ok=%v err=%v", ok, err)
	}
	runJSON, _ := json.MarshalIndent(run, "", "  ")
	startFixture := h.writeFixture("happy/start.json", []byte(startOut))
	doctorFixture := h.writeFixture("happy/doctor.json", []byte(doctorOut))
	manifestFixture := h.writeFixture("happy/run-manifest.json", append(runJSON, '\n'))
	t.Logf("happy-path fixtures: %s, %s, %s", startFixture, doctorFixture, manifestFixture)
}

func TestFakeRepoHarnessFailureRetryIntegration(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-903", 903, ticket.State("todo"), []ticket.AcceptanceCriterion{
		{Description: "ready file exists", Run: "test -f .ready"},
	})
	h.t.Setenv("DOCKET_HOOK_AC_ENFORCE", "1")

	failOut, err := h.run("__hook-ac-check", "TKT-903")
	if err == nil {
		t.Fatalf("expected first hook check to fail, output=%s", failOut)
	}
	if !strings.Contains(failOut, "AC 1 failed") {
		t.Fatalf("expected deterministic failure trace, got: %s", failOut)
	}

	if err := os.WriteFile(filepath.Join(h.repo, ".ready"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write retry marker failed: %v", err)
	}
	retryOut, err := h.run("__hook-ac-check", "TKT-903")
	if err != nil {
		t.Fatalf("expected retry to pass, err=%v output=%s", err, retryOut)
	}
	if !strings.Contains(retryOut, "AC 1 passed") {
		t.Fatalf("expected deterministic success trace, got: %s", retryOut)
	}

	failFixture := h.writeFixture("failure-retry/fail-trace.txt", []byte(failOut))
	retryFixture := h.writeFixture("failure-retry/retry-trace.txt", []byte(retryOut))
	t.Logf("failure-retry fixtures: %s, %s", failFixture, retryFixture)
}

type adapterMatrixFixture struct {
	AdapterID             string
	ExpectedRepoArtifacts []string
}

func buildAdapterMatrixFixtures() []adapterMatrixFixture {
	fixtures := []adapterMatrixFixture{
		{
			AdapterID:             "codex",
			ExpectedRepoArtifacts: []string{"AGENTS.md", ".git/hooks/pre-commit", "CLAUDE.md", ".docket/install.json"},
		},
		{
			AdapterID:             "claude-code",
			ExpectedRepoArtifacts: []string{"CLAUDE.md", ".git/hooks/pre-commit", ".docket/install.json"},
		},
		{
			AdapterID:             "gemini",
			ExpectedRepoArtifacts: []string{"GEMINI.md", ".git/hooks/pre-commit", "CLAUDE.md", ".docket/install.json"},
		},
	}
	return fixtures
}

func TestAdapterMatrixFixtureBuilders(t *testing.T) {
	fixtures := buildAdapterMatrixFixtures()
	if len(fixtures) != 3 {
		t.Fatalf("expected 3 matrix fixtures, got %d", len(fixtures))
	}
	gotIDs := []string{fixtures[0].AdapterID, fixtures[1].AdapterID, fixtures[2].AdapterID}
	sort.Strings(gotIDs)
	if strings.Join(gotIDs, ",") != "claude-code,codex,gemini" {
		t.Fatalf("unexpected adapter fixture ids: %v", gotIDs)
	}
	for _, fixture := range fixtures {
		if len(fixture.ExpectedRepoArtifacts) == 0 {
			t.Fatalf("expected artifact patterns for %s", fixture.AdapterID)
		}
	}
}

func TestAdapterMatrixIntegration(t *testing.T) {
	fixtures := buildAdapterMatrixFixtures()

	for i, fixture := range fixtures {
		h := newFakeRepoHarnessForAdapter(t, fixture.AdapterID)
		ticketID := fmt.Sprintf("TKT-%03d", 920+i)
		h.seedTicket(ticketID, 920+i, ticket.State("todo"), []ticket.AcceptanceCriterion{{Description: "ac"}})

		doctorBeforeOut, err := h.run("doctor", "--format", "json")
		if err != nil {
			t.Fatalf("%s: doctor before bootstrap failed: %v\n%s", fixture.AdapterID, err, doctorBeforeOut)
		}
		var before doctorReport
		if err := json.Unmarshal([]byte(doctorBeforeOut), &before); err != nil {
			t.Fatalf("%s: unmarshal doctor before failed: %v\n%s", fixture.AdapterID, err, doctorBeforeOut)
		}
		if statusByName(before.Checks, "hooks") != "FAIL" {
			t.Fatalf("%s: expected hooks FAIL before bootstrap", fixture.AdapterID)
		}

		if out, err := h.run("bootstrap", "--adapter", fixture.AdapterID); err != nil {
			t.Fatalf("%s: bootstrap failed: %v\n%s", fixture.AdapterID, err, out)
		}
		for _, rel := range fixture.ExpectedRepoArtifacts {
			if _, err := os.Stat(filepath.Join(h.repo, rel)); err != nil {
				t.Fatalf("%s: expected artifact %s after bootstrap: %v", fixture.AdapterID, rel, err)
			}
		}
		// Gemini fixture keeps a dedicated marker; remove CLAUDE.md created by bootstrap
		// so adapter detection stays pinned to gemini in this matrix scenario.
		if fixture.AdapterID == "gemini" {
			if err := os.Remove(filepath.Join(h.repo, "CLAUDE.md")); err != nil {
				t.Fatalf("gemini: failed to remove CLAUDE.md disambiguator: %v", err)
			}
		}

		startOut, err := h.run("start", "--format", "json")
		if err != nil {
			t.Fatalf("%s: start failed: %v\n%s", fixture.AdapterID, err, startOut)
		}
		var startPayload map[string]any
		if err := json.Unmarshal([]byte(startOut), &startPayload); err != nil {
			t.Fatalf("%s: unmarshal start payload failed: %v\n%s", fixture.AdapterID, err, startOut)
		}
		capDigest, ok := startPayload["capability_digest"].(map[string]any)
		if !ok {
			t.Fatalf("%s: expected capability_digest object in start output", fixture.AdapterID)
		}
		if capDigest["adapter"] != fixture.AdapterID {
			t.Fatalf("%s: expected capability adapter %q, got %v", fixture.AdapterID, fixture.AdapterID, capDigest["adapter"])
		}

		doctorAfterOut, err := h.run("doctor", "--format", "json")
		if err != nil {
			t.Fatalf("%s: doctor after bootstrap failed: %v\n%s", fixture.AdapterID, err, doctorAfterOut)
		}
		var after doctorReport
		if err := json.Unmarshal([]byte(doctorAfterOut), &after); err != nil {
			t.Fatalf("%s: unmarshal doctor after failed: %v\n%s", fixture.AdapterID, err, doctorAfterOut)
		}
		if after.Adapter != fixture.AdapterID {
			t.Fatalf("%s: expected doctor adapter %q, got %q", fixture.AdapterID, fixture.AdapterID, after.Adapter)
		}
		if statusByName(after.Checks, "hooks") != "PASS" {
			t.Fatalf("%s: expected hooks PASS after bootstrap", fixture.AdapterID)
		}

		ns := security.NewRepoNamespaceStore(h.home)
		run, ok, err := ns.GetRunManifest(h.repo, ticketID)
		if err != nil || !ok {
			t.Fatalf("%s: expected run manifest for %s, ok=%v err=%v", fixture.AdapterID, ticketID, ok, err)
		}
		runJSON, _ := json.MarshalIndent(run, "", "  ")
		startFixture := h.writeFixture(filepath.Join("matrix", fixture.AdapterID, "start.json"), []byte(startOut))
		doctorBeforeFixture := h.writeFixture(filepath.Join("matrix", fixture.AdapterID, "doctor-before.json"), []byte(doctorBeforeOut))
		doctorAfterFixture := h.writeFixture(filepath.Join("matrix", fixture.AdapterID, "doctor-after.json"), []byte(doctorAfterOut))
		manifestFixture := h.writeFixture(filepath.Join("matrix", fixture.AdapterID, "run-manifest.json"), append(runJSON, '\n'))
		t.Logf("%s fixtures: %s | %s | %s | %s", fixture.AdapterID, startFixture, doctorBeforeFixture, doctorAfterFixture, manifestFixture)
	}
}
