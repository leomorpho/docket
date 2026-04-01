package ticket

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	docketgit "github.com/leomorpho/docket/internal/git"
)

func TestDefaultConfigSemanticDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Semantic.Provider != "uv" {
		t.Fatalf("provider = %q", cfg.Semantic.Provider)
	}
	if cfg.Semantic.Model == "" {
		t.Fatal("expected default semantic model")
	}
	if !strings.Contains(cfg.Semantic.HFHome, filepath.Join(".cache", "docket", "hf")) {
		t.Fatalf("hf home = %q", cfg.Semantic.HFHome)
	}
	if cfg.Semantic.LexicalWeight != 0.35 || cfg.Semantic.VectorWeight != 0.65 {
		t.Fatalf("unexpected weights: %+v", cfg.Semantic)
	}
	if cfg.Semantic.TitleWeight != 3.0 || cfg.Semantic.DescriptionWeight != 1.5 || cfg.Semantic.ACWeight != 2.0 || cfg.Semantic.HandoffWeight != 1.25 {
		t.Fatalf("unexpected field weights: %+v", cfg.Semantic)
	}
}

func TestDefaultConfigStateDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DefaultState != "draft" {
		t.Fatalf("default_state = %q, want draft", cfg.DefaultState)
	}
	if cfg.DefaultPriority != 10 {
		t.Fatalf("default_priority = %d, want 10", cfg.DefaultPriority)
	}
	if len(cfg.HandoffSections) == 0 {
		t.Fatal("expected handoff_sections to be populated")
	}
	if len(cfg.States) != 5 {
		t.Fatalf("expected 5 default states, got %d", len(cfg.States))
	}

	draft, ok := cfg.States["draft"]
	if !ok {
		t.Fatal("missing draft state")
	}
	if !draft.Open {
		t.Error("draft should be open")
	}
	if draft.Column != 0 {
		t.Errorf("draft column = %d, want 0", draft.Column)
	}

	validated, ok := cfg.States["validated"]
	if !ok {
		t.Fatal("missing validated state")
	}
	if validated.Open {
		t.Error("validated should not be open")
	}
	if cfg.Workflow.Version != 1 {
		t.Fatalf("workflow.version = %d, want 1", cfg.Workflow.Version)
	}
	if len(cfg.Workflow.States) != len(cfg.States) {
		t.Fatalf("workflow states = %d, want %d", len(cfg.Workflow.States), len(cfg.States))
	}
}

func TestDefaultConfigUsesShippedWorkflowDefinition(t *testing.T) {
	cfg := DefaultConfig()
	workflow := defaultWorkflowConfig()
	if !reflect.DeepEqual(cfg.Workflow, workflow) {
		t.Fatalf("DefaultConfig workflow should match shipped workflow definition")
	}
	derived := statesFromWorkflow(workflow)
	if !reflect.DeepEqual(cfg.States, derived) {
		t.Fatalf("DefaultConfig states should be derived from shipped workflow definition")
	}
}

func TestMigrateStateNamesUsesShippedWorkflowDefaultsForKnownStates(t *testing.T) {
	migrated := migrateStateNames([]string{"draft", "ready", "running", "validated", "archived"})
	expected := statesFromWorkflow(defaultWorkflowConfig())
	for _, name := range []string{"draft", "ready", "running", "validated", "archived"} {
		if !reflect.DeepEqual(migrated[name], expected[name]) {
			t.Fatalf("migrateStateNames(%s) should use shipped defaults", name)
		}
	}
}

func TestLoadConfigWorkflowV1Schema(t *testing.T) {
	tmpDir := t.TempDir()
	raw := `{
  "counter": 1,
  "backend": "local",
  "default_state": "queued",
  "workflow": {
    "version": 1,
    "states": {
      "queued": {
        "semantics": {
          "roles": ["intake"],
          "open": true,
          "startable": true,
          "blocks_dependents": true,
          "next": ["building"]
        },
        "presentation": {
          "label": "Queued",
          "column": 0
        }
      },
      "building": {
        "semantics": {
          "roles": ["active"],
          "open": true,
          "blocks_dependents": true,
          "next": ["review"]
        },
        "presentation": {
          "label": "Building",
          "column": 1
        }
      },
      "review": {
        "semantics": {
          "roles": ["review"],
          "open": true,
          "reviewable": true,
          "blocks_dependents": false,
          "next": ["shipped"]
        },
        "presentation": {
          "label": "Review",
          "column": 2
        }
      },
      "shipped": {
        "semantics": {
          "roles": ["completed"],
          "open": false,
          "terminal": true,
          "next": []
        },
        "presentation": {
          "label": "Shipped",
          "column": 3
        }
      }
    }
  },
  "labels": ["feature"]
}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(tmpDir), []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Workflow.Version != 1 {
		t.Fatalf("workflow.version = %d, want 1", cfg.Workflow.Version)
	}
	if !cfg.IsValidState("queued") || !cfg.IsValidState("review") {
		t.Fatalf("expected workflow states to populate state lookup, got %#v", cfg.StateNames())
	}
	queued := cfg.States["queued"]
	if !queued.Startable {
		t.Fatal("queued should be startable")
	}
	if len(queued.Roles) != 1 || queued.Roles[0] != "intake" {
		t.Fatalf("queued roles = %#v", queued.Roles)
	}
	review := cfg.States["review"]
	if !review.Reviewable {
		t.Fatal("review should be reviewable")
	}
	if review.BlocksDependents {
		t.Fatal("review should not block dependents")
	}
	shipped := cfg.States["shipped"]
	if !shipped.Terminal {
		t.Fatal("shipped should be terminal")
	}
	if shipped.Open {
		t.Fatal("shipped should not be open")
	}
	if got := cfg.ValidTransitions("queued"); len(got) != 1 || got[0] != "building" {
		t.Fatalf("queued transitions = %#v", got)
	}
	if cfg.BlocksDependents("review") {
		t.Fatal("review should not block dependents via helper")
	}
	if !cfg.BlocksDependents("queued") {
		t.Fatal("queued should block dependents via helper")
	}
}

func TestConfigPathUsesSharedRootFromWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	runGitConfig(t, tmpDir, "init")
	runGitConfig(t, tmpDir, "config", "user.email", "test@example.com")
	runGitConfig(t, tmpDir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(tmpDir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	runGitConfig(t, tmpDir, "add", ".")
	runGitConfig(t, tmpDir, "commit", "-m", "seed")

	worktreePath := filepath.Join(tmpDir, "wt")
	runGitConfig(t, tmpDir, "worktree", "add", "-b", "docket/test-config", worktreePath)

	sharedRoot := docketgit.SharedRepoRoot(tmpDir)
	if got, want := ConfigPath(worktreePath), filepath.Join(sharedRoot, ".docket", "config.json"); got != want {
		t.Fatalf("ConfigPath(worktree) = %q, want %q", got, want)
	}
}

func TestSaveConfigPreservesFalseWorkflowBooleans(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	validated := cfg.States["validated"]
	validated.BlocksDependents = false
	validated.Reviewable = false
	cfg.States["validated"] = validated

	if err := SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	reloaded := loaded.States["validated"]
	if reloaded.BlocksDependents {
		t.Fatal("BlocksDependents should remain false after save/load")
	}
	if reloaded.Reviewable {
		t.Fatal("Reviewable should remain false after save/load")
	}
}

func TestDefaultConfigDoesNotBlockDependentsInValidated(t *testing.T) {
	cfg := DefaultConfig()

	validated := cfg.States["validated"]
	if validated.BlocksDependents {
		t.Fatal("default validated state should not block dependents")
	}
	if validated.Reviewable {
		t.Fatal("default validated state should not be reviewable")
	}
	if cfg.BlocksDependents("validated") {
		t.Fatal("helper should report default validated as non-blocking")
	}
}

func TestLoadConfigWorkflowV1SchemaValidation(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		substr string
	}{
		{
			name: "unknown role",
			raw: `{
  "default_state": "queued",
  "workflow": {
    "version": 1,
    "states": {
      "queued": {
        "semantics": {"roles": ["mystery"], "open": true, "next": []},
        "presentation": {"label": "Queued", "column": 0}
      }
    }
  }
}`,
			substr: "workflow.states.queued.semantics.roles[0]",
		},
		{
			name: "missing label",
			raw: `{
  "default_state": "queued",
  "workflow": {
    "version": 1,
    "states": {
      "queued": {
        "semantics": {"roles": ["intake"], "open": true, "next": []},
        "presentation": {"column": 0}
      }
    }
  }
}`,
			substr: "workflow.states.queued.presentation.label",
		},
		{
			name: "negative column",
			raw: `{
  "default_state": "queued",
  "workflow": {
    "version": 1,
    "states": {
      "queued": {
        "semantics": {"roles": ["intake"], "open": true, "next": []},
        "presentation": {"label": "Queued", "column": -1}
      }
    }
  }
}`,
			substr: "workflow.states.queued.presentation.column",
		},
		{
			name: "unknown transition target",
			raw: `{
  "default_state": "queued",
  "workflow": {
    "version": 1,
    "states": {
      "queued": {
        "semantics": {"roles": ["intake"], "open": true, "next": ["missing"]},
        "presentation": {"label": "Queued", "column": 0}
      }
    }
  }
}`,
			substr: "workflow.states.queued.semantics.next[0]",
		},
		{
			name: "terminal cannot be open",
			raw: `{
  "default_state": "queued",
  "workflow": {
    "version": 1,
    "states": {
      "queued": {
        "semantics": {"roles": ["completed"], "open": true, "terminal": true, "next": []},
        "presentation": {"label": "Queued", "column": 0}
      }
    }
  }
}`,
			substr: "workflow.states.queued.semantics.terminal",
		},
		{
			name: "startable must be open",
			raw: `{
  "default_state": "queued",
  "workflow": {
    "version": 1,
    "states": {
      "queued": {
        "semantics": {"roles": ["intake"], "open": false, "startable": true, "next": []},
        "presentation": {"label": "Queued", "column": 0}
      }
    }
  }
}`,
			substr: "workflow.states.queued.semantics.startable",
		},
		{
			name: "startable must lead to active state",
			raw: `{
  "default_state": "queued",
  "workflow": {
    "version": 1,
    "states": {
      "queued": {
        "semantics": {"roles": ["intake"], "open": true, "startable": true, "next": ["qa"]},
        "presentation": {"label": "Queued", "column": 0}
      },
      "qa": {
        "semantics": {"roles": ["review"], "open": true, "reviewable": true, "next": []},
        "presentation": {"label": "QA", "column": 1}
      }
    }
  }
}`,
			substr: "workflow.states.queued.semantics.next",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(ConfigPath(tmpDir), []byte(tc.raw), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			_, err := LoadConfig(tmpDir)
			if err == nil || !strings.Contains(err.Error(), tc.substr) {
				t.Fatalf("expected error containing %q, got %v", tc.substr, err)
			}
		})
	}
}

func runGitConfig(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func TestConfigStartTransitionTargetsAndRolesUseWorkflowSemantics(t *testing.T) {
	tmpDir := t.TempDir()
	raw := `{
  "default_state": "queued",
  "workflow": {
    "version": 1,
    "states": {
      "queued": {
        "semantics": {
          "roles": ["intake"],
          "open": true,
          "startable": true,
          "next": ["building", "qa"]
        },
        "presentation": {
          "label": "Queued",
          "column": 0
        }
      },
      "building": {
        "semantics": {
          "roles": ["active"],
          "open": true,
          "next": ["qa"]
        },
        "presentation": {
          "label": "Building",
          "column": 1
        }
      },
      "qa": {
        "semantics": {
          "roles": ["review"],
          "open": true,
          "reviewable": true,
          "next": ["shipped"]
        },
        "presentation": {
          "label": "QA",
          "column": 2
        }
      },
      "shipped": {
        "semantics": {
          "roles": ["completed"],
          "open": false,
          "terminal": true,
          "next": []
        },
        "presentation": {
          "label": "Shipped",
          "column": 3
        }
      }
    }
  }
}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(tmpDir), []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if got := cfg.StatesWithRole("active"); len(got) != 1 || got[0] != "building" {
		t.Fatalf("StatesWithRole(active) = %#v", got)
	}
	if got := cfg.StartTransitionTargets("queued"); len(got) != 1 || got[0] != "building" {
		t.Fatalf("StartTransitionTargets(queued) = %#v", got)
	}
	if got, ok := cfg.PrimaryStateWithRole("review"); !ok || got != "qa" {
		t.Fatalf("PrimaryStateWithRole(review) = %q, %v", got, ok)
	}
	if !cfg.StateHasRole("shipped", "completed") {
		t.Fatal("expected shipped to carry completed role")
	}
}

// TestLoadConfigMigratesOldArrayFormat verifies that a config with the legacy
// []string states format is auto-migrated to the new map format on load.
func TestLoadConfigMigratesOldArrayFormat(t *testing.T) {
	tmpDir := t.TempDir()
	raw := `{"counter":1,"backend":"local","states":["backlog","todo","in-progress","in-review","done","archived"],"labels":["bug"],"commit_sessions":false}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(tmpDir), []byte(raw), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// States should be migrated to map format.
	if len(cfg.States) != 6 {
		t.Fatalf("expected 6 states after migration, got %d", len(cfg.States))
	}
	if _, ok := cfg.States["backlog"]; !ok {
		t.Error("backlog state missing after migration")
	}
	if cfg.States["backlog"].Open != true {
		t.Error("backlog should be open after migration")
	}

	// Verify the on-disk config was migrated (saved in new format).
	data, err := os.ReadFile(ConfigPath(tmpDir))
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	var onDisk map[string]json.RawMessage
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("parse migrated config: %v", err)
	}
	statesRaw := strings.TrimSpace(string(onDisk["states"]))
	if len(statesRaw) == 0 || statesRaw[0] != '{' {
		t.Errorf("migrated config states should be object, got: %s", statesRaw)
	}
}

// TestLoadConfigNewMapFormat verifies that a config already in the new map format
// loads without errors.
func TestLoadConfigNewMapFormat(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := SaveConfig(tmpDir, DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.States) != 5 {
		t.Fatalf("expected 5 states, got %d", len(cfg.States))
	}
}

func TestLoadConfigLegacyCanonicalMapBackfillsSemanticRoles(t *testing.T) {
	tmpDir := t.TempDir()
	raw := `{
  "counter": 1,
  "backend": "local",
  "states": {
    "backlog": {
      "label": "Backlog",
      "open": true,
      "column": 0,
      "next": ["todo", "in-progress", "archived"],
      "startable": true
    },
    "todo": {
      "label": "To Do",
      "open": true,
      "column": 1,
      "next": ["in-progress", "backlog", "archived"],
      "startable": true
    },
    "in-progress": {
      "label": "In Progress",
      "open": true,
      "column": 2,
      "next": ["in-review", "todo", "archived"]
    },
    "in-review": {
      "label": "In Review",
      "open": true,
      "column": 3,
      "next": ["done", "in-progress", "archived"]
    },
    "done": {
      "label": "Done",
      "open": false,
      "column": 4,
      "next": ["archived", "in-progress"]
    },
    "archived": {
      "label": "Archived",
      "open": false,
      "column": 5,
      "next": ["backlog"]
    }
  },
  "labels": ["bug"],
  "commit_sessions": false
}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(tmpDir), []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg.StateHasRole("in-progress", "active") {
		t.Fatal("expected in-progress to regain active role semantics")
	}
	if !cfg.StateHasRole("done", "completed") {
		t.Fatal("expected done to regain completed role semantics")
	}
	if got := cfg.StartTransitionTargets("backlog"); len(got) != 1 || got[0] != "in-progress" {
		t.Fatalf("StartTransitionTargets(backlog) = %#v, want [\"in-progress\"]", got)
	}
	if !cfg.BlocksDependents("in-progress") {
		t.Fatal("expected in-progress to block dependents after semantic backfill")
	}
}

func TestLoadConfigLegacyMapAutoAddsDirectActiveTransitionForStartableState(t *testing.T) {
	tmpDir := t.TempDir()
	raw := `{
  "counter": 1,
  "backend": "local",
  "states": {
    "backlog": {
      "label": "Backlog",
      "open": true,
      "column": 0,
      "next": ["todo", "archived"],
      "roles": ["intake"],
      "startable": true,
      "terminal": false,
      "reviewable": false,
      "blocks_dependents": true
    },
    "todo": {
      "label": "To Do",
      "open": true,
      "column": 1,
      "next": ["in-progress", "backlog", "archived"],
      "roles": ["intake"],
      "startable": true,
      "terminal": false,
      "reviewable": false,
      "blocks_dependents": true
    },
    "in-progress": {
      "label": "In Progress",
      "open": true,
      "column": 2,
      "next": ["in-review", "todo", "archived"],
      "roles": ["active"],
      "startable": false,
      "terminal": false,
      "reviewable": false,
      "blocks_dependents": true
    },
    "in-review": {
      "label": "In Review",
      "open": true,
      "column": 3,
      "next": ["done", "in-progress", "archived"],
      "roles": ["review"],
      "startable": false,
      "terminal": false,
      "reviewable": true,
      "blocks_dependents": true
    },
    "done": {
      "label": "Done",
      "open": false,
      "column": 4,
      "next": ["archived", "in-progress"],
      "roles": ["completed"],
      "startable": false,
      "terminal": true,
      "reviewable": false,
      "blocks_dependents": false
    },
    "archived": {
      "label": "Archived",
      "open": false,
      "column": 5,
      "next": ["backlog"],
      "roles": ["archived"],
      "startable": false,
      "terminal": true,
      "reviewable": false,
      "blocks_dependents": false
    }
  },
  "labels": ["bug"],
  "commit_sessions": false
}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(tmpDir), []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	backlog := cfg.States["backlog"]
	if len(backlog.Next) == 0 || backlog.Next[0] != "in-progress" {
		t.Fatalf("expected compatibility migration to prepend in-progress, got %#v", backlog.Next)
	}
	targets := cfg.StartTransitionTargets("backlog")
	if len(targets) == 0 || targets[0] != "in-progress" {
		t.Fatalf("StartTransitionTargets(backlog) = %#v, want in-progress first", targets)
	}

	data, err := os.ReadFile(ConfigPath(tmpDir))
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	var onDisk Config
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("parse migrated config: %v", err)
	}
	if got := onDisk.States["backlog"].Next; len(got) == 0 || got[0] != "in-progress" {
		t.Fatalf("expected persisted compatibility migration, got %#v", got)
	}
}

func TestLoadConfigLegacyMapKeepsExistingDirectActiveTransition(t *testing.T) {
	tmpDir := t.TempDir()
	raw := `{
  "counter": 1,
  "backend": "local",
  "states": {
    "backlog": {
      "label": "Backlog",
      "open": true,
      "column": 0,
      "next": ["in-progress", "todo", "archived"],
      "roles": ["intake"],
      "startable": true
    },
    "todo": {
      "label": "To Do",
      "open": true,
      "column": 1,
      "next": ["in-progress", "backlog", "archived"],
      "roles": ["intake"],
      "startable": true
    },
    "in-progress": {
      "label": "In Progress",
      "open": true,
      "column": 2,
      "next": ["in-review", "todo", "archived"],
      "roles": ["active"]
    },
    "in-review": {
      "label": "In Review",
      "open": true,
      "column": 3,
      "next": ["done", "in-progress", "archived"],
      "roles": ["review"]
    },
    "done": {
      "label": "Done",
      "open": false,
      "column": 4,
      "next": ["archived", "in-progress"],
      "roles": ["completed"],
      "terminal": true
    },
    "archived": {
      "label": "Archived",
      "open": false,
      "column": 5,
      "next": ["backlog"],
      "roles": ["archived"],
      "terminal": true
    }
  },
  "labels": ["bug"],
  "commit_sessions": false
}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(tmpDir), []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	got := cfg.States["backlog"].Next
	if len(got) < 3 || got[0] != "in-progress" || got[1] != "todo" {
		t.Fatalf("expected backlog next order to remain stable, got %#v", got)
	}
}

// TestLoadConfigMigratesUnknownState verifies that unknown state names in the
// old array format get sensible StateConfig defaults.
func TestLoadConfigMigratesUnknownState(t *testing.T) {
	tmpDir := t.TempDir()
	raw := `{"counter":0,"backend":"local","states":["backlog","stale","done"],"labels":[]}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(tmpDir), []byte(raw), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	stale, ok := cfg.States["stale"]
	if !ok {
		t.Fatal("stale state missing")
	}
	if stale.Label == "" {
		t.Error("stale state should have a label")
	}
}

// TestConfigHelpers validates StateNames, OpenStates, IsValidState, ValidTransitions, ColumnOrder.
func TestConfigHelpers(t *testing.T) {
	cfg := DefaultConfig()

	// IsValidState
	if !cfg.IsValidState("draft") {
		t.Error("draft should be valid")
	}
	if cfg.IsValidState("nonexistent") {
		t.Error("nonexistent should not be valid")
	}

	// OpenStates: should include draft, ready, running; exclude validated, archived
	open := cfg.OpenStates()
	openSet := make(map[string]bool, len(open))
	for _, s := range open {
		openSet[s] = true
	}
	for _, s := range []string{"draft", "ready", "running"} {
		if !openSet[s] {
			t.Errorf("expected %q in open states", s)
		}
	}
	for _, s := range []string{"validated", "archived"} {
		if openSet[s] {
			t.Errorf("did not expect %q in open states", s)
		}
	}

	// OpenStates should be sorted by column.
	for i := 1; i < len(open); i++ {
		if cfg.States[open[i]].Column < cfg.States[open[i-1]].Column {
			t.Errorf("OpenStates not sorted by column at index %d", i)
		}
	}

	startable := cfg.StartableStates()
	wantStartable := []string{"ready"}
	if len(startable) != len(wantStartable) {
		t.Fatalf("StartableStates length = %d, want %d (%v)", len(startable), len(wantStartable), startable)
	}
	for i, want := range wantStartable {
		if startable[i] != want {
			t.Fatalf("StartableStates[%d] = %q, want %q", i, startable[i], want)
		}
	}

	// ValidTransitions
	next := cfg.ValidTransitions("draft")
	if len(next) == 0 {
		t.Error("expected transitions from draft")
	}
	found := false
	for _, s := range next {
		if s == "ready" {
			found = true
		}
	}
	if !found {
		t.Error("expected draft -> ready transition")
	}
	if cfg.ValidTransitions("nonexistent") != nil {
		t.Error("expected nil for unknown state")
	}

	// ColumnOrder: should return all states sorted by Column.
	cols := cfg.ColumnOrder()
	if len(cols) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(cols))
	}
	for i := 1; i < len(cols); i++ {
		if cols[i].Column < cols[i-1].Column {
			t.Errorf("ColumnOrder not sorted at index %d", i)
		}
	}

	// StateNames: should return keys sorted by Column.
	names := cfg.StateNames()
	if len(names) != 5 {
		t.Fatalf("expected 5 state names, got %d", len(names))
	}
	if names[0] != "draft" {
		t.Errorf("first state should be draft (column 0), got %q", names[0])
	}
}

// TestDefaultStateAndPriorityApplyDefaults verifies missing fields get defaults.
func TestDefaultStateAndPriorityApplyDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	// Config without default_state or default_priority.
	raw := `{"counter":0,"backend":"local","states":{"backlog":{"label":"Backlog","open":true,"column":0,"next":["todo"]}},"labels":[]}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(tmpDir), []byte(raw), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DefaultState != "draft" {
		t.Errorf("default_state = %q, want draft", cfg.DefaultState)
	}
	if cfg.DefaultPriority != 10 {
		t.Errorf("default_priority = %d, want 10", cfg.DefaultPriority)
	}
	if len(cfg.HandoffSections) == 0 {
		t.Error("handoff_sections should be defaulted")
	}
}

func TestLoadConfigAppliesSemanticDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	raw := `{"counter":1,"backend":"local","states":["backlog"],"labels":["bug"],"commit_sessions":false}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(tmpDir), []byte(raw), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Semantic.Provider != "uv" {
		t.Fatalf("expected default provider, got %q", cfg.Semantic.Provider)
	}
	if cfg.Semantic.Model == "" || cfg.Semantic.HFHome == "" || cfg.Semantic.UVCacheDir == "" {
		t.Fatalf("expected semantic defaults, got %+v", cfg.Semantic)
	}
}

func TestLoadConfigSemanticEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Semantic.Provider = "config-provider"
	cfg.Semantic.Model = "config-model"
	cfg.Semantic.HFHome = "/config/hf"
	cfg.Semantic.LexicalWeight = 0.2
	cfg.Semantic.VectorWeight = 0.8
	cfg.Semantic.TitleWeight = 4
	if err := SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	t.Setenv("DOCKET_SEMANTIC_ENABLED", "true")
	t.Setenv("DOCKET_SEMANTIC_PROVIDER", "uv")
	t.Setenv("DOCKET_SEMANTIC_MODEL", "env-model")
	t.Setenv("DOCKET_SEMANTIC_HF_HOME", "/env/hf")
	t.Setenv("DOCKET_SEMANTIC_SENTENCE_TRANSFORMERS_HOME", "/env/sbert")
	t.Setenv("DOCKET_SEMANTIC_UV_CACHE_DIR", "/env/uv")
	t.Setenv("DOCKET_SEMANTIC_LEXICAL_WEIGHT", "0.4")
	t.Setenv("DOCKET_SEMANTIC_VECTOR_WEIGHT", "0.6")
	t.Setenv("DOCKET_SEMANTIC_TITLE_WEIGHT", "5")
	t.Setenv("DOCKET_SEMANTIC_DESCRIPTION_WEIGHT", "1.2")
	t.Setenv("DOCKET_SEMANTIC_AC_WEIGHT", "2.4")
	t.Setenv("DOCKET_SEMANTIC_HANDOFF_WEIGHT", "0.9")

	loaded, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !loaded.Semantic.Enabled {
		t.Fatal("expected enabled override")
	}
	if loaded.Semantic.Provider != "uv" || loaded.Semantic.Model != "env-model" {
		t.Fatalf("unexpected provider/model: %+v", loaded.Semantic)
	}
	if loaded.Semantic.HFHome != "/env/hf" || loaded.Semantic.SentenceTransformersHome != "/env/sbert" || loaded.Semantic.UVCacheDir != "/env/uv" {
		t.Fatalf("unexpected cache paths: %+v", loaded.Semantic)
	}
	if loaded.Semantic.LexicalWeight != 0.4 || loaded.Semantic.VectorWeight != 0.6 {
		t.Fatalf("unexpected weights: %+v", loaded.Semantic)
	}
	if loaded.Semantic.TitleWeight != 5 || loaded.Semantic.DescriptionWeight != 1.2 || loaded.Semantic.ACWeight != 2.4 || loaded.Semantic.HandoffWeight != 0.9 {
		t.Fatalf("unexpected field weights: %+v", loaded.Semantic)
	}
}

func TestLoadConfigSemanticInvalidEnv(t *testing.T) {
	tmpDir := t.TempDir()
	if err := SaveConfig(tmpDir, DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	t.Setenv("DOCKET_SEMANTIC_VECTOR_WEIGHT", "bad")

	_, err := LoadConfig(tmpDir)
	if err == nil || !strings.Contains(err.Error(), "DOCKET_SEMANTIC_VECTOR_WEIGHT") {
		t.Fatalf("expected env parse error, got %v", err)
	}
}

func TestLoadConfigSecurityEnforcementFlag(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.SecurityEnforcement = true
	if err := SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !loaded.SecurityEnforcement {
		t.Fatal("expected security_enforcement=true after save/load")
	}
}

func TestLoadConfigSecurityEnforcementDefaultsFalseWhenOmitted(t *testing.T) {
	tmpDir := t.TempDir()
	raw := `{"counter":1,"backend":"local","states":{"backlog":{"label":"Backlog","open":true,"column":0,"next":["todo"]},"todo":{"label":"Todo","open":true,"column":1,"next":["done"]},"done":{"label":"Done","open":false,"column":2,"next":[]}},"labels":["bug"],"commit_sessions":false,"default_state":"backlog","default_priority":1}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(tmpDir), []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loaded, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.SecurityEnforcement {
		t.Fatal("expected security_enforcement default false when field is omitted")
	}
}

func TestLoadConfigSecurityEnforcementIgnoresLocalEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.SecurityEnforcement = false
	if err := SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	t.Setenv("DOCKET_HOOK_AC_ENFORCE", "1")
	t.Setenv("DOCKET_ENFORCE_HOOKS", "1")

	loaded, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.SecurityEnforcement {
		t.Fatal("expected repo config to remain source of truth for security enforcement")
	}
}
