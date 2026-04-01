package ticket

import (
	"reflect"
	"testing"
)

func TestMigrateConfigToNorthStarRewritesLegacyStates(t *testing.T) {
	cfg := &Config{
		Backend: "local",
		States: map[string]StateConfig{
			"backlog":     legacyDefaultStates["backlog"],
			"todo":        legacyDefaultStates["todo"],
			"in-progress": legacyDefaultStates["in-progress"],
			"in-review":   legacyDefaultStates["in-review"],
			"done":        legacyDefaultStates["done"],
			"archived":    legacyDefaultStates["archived"],
		},
		DefaultState: "backlog",
	}

	migrated, changed := MigrateConfigToNorthStar(cfg)
	if !changed {
		t.Fatal("expected legacy config to require migration")
	}
	for _, name := range []string{"backlog", "todo", "in-progress", "in-review", "done"} {
		if _, ok := migrated.States[name]; ok {
			t.Fatalf("expected legacy state %s to be removed", name)
		}
	}
	for _, name := range canonicalNorthStarStateNames() {
		if _, ok := migrated.States[name]; !ok {
			t.Fatalf("expected canonical state %s after migration", name)
		}
	}
	if migrated.DefaultState != "draft" {
		t.Fatalf("default state = %q, want draft", migrated.DefaultState)
	}
	if !reflect.DeepEqual(migrated.States["archived"], DefaultConfig().States["archived"]) {
		t.Fatalf("expected archived state to match north-star default, got %#v", migrated.States["archived"])
	}
}

func TestMigrateConfigToNorthStarPreservesCustomStatesAndExistingNorthStarConfig(t *testing.T) {
	cfg := DefaultConfig()
	draft := cfg.States["draft"]
	draft.Label = "Draft Work"
	cfg.States["draft"] = draft
	cfg.States["stale"] = StateConfig{
		Label:            "Stale",
		Open:             false,
		Column:           5,
		Next:             []string{"todo", "archived"},
		Terminal:         false,
		Startable:        false,
		Reviewable:       false,
		BlocksDependents: false,
	}

	migrated, changed := MigrateConfigToNorthStar(cfg)
	if !changed {
		t.Fatal("expected custom transition normalization to report a migration change")
	}
	if migrated.States["draft"].Label != "Draft Work" {
		t.Fatalf("expected existing north-star draft config to be preserved, got %#v", migrated.States["draft"])
	}
	stale, ok := migrated.States["stale"]
	if !ok {
		t.Fatal("expected custom stale state to be preserved")
	}
	if len(stale.Next) != 2 || stale.Next[0] != "ready" || stale.Next[1] != "archived" {
		t.Fatalf("expected custom stale transitions to migrate legacy names, got %#v", stale.Next)
	}
}
