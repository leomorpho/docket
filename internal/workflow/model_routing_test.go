package workflow

import "testing"

type fixtureProviderAdapter struct {
	name   string
	models map[CapabilityTier][]ModelSpec
}

func (f fixtureProviderAdapter) ProviderName() string {
	return f.name
}

func (f fixtureProviderAdapter) ModelsByTier() map[CapabilityTier][]ModelSpec {
	return f.models
}

func TestParseCapabilityTier(t *testing.T) {
	tests := []struct {
		in   string
		want CapabilityTier
		ok   bool
	}{
		{in: "cheap", want: TierCheap, ok: true},
		{in: "balanced", want: TierBalanced, ok: true},
		{in: "strong", want: TierStrong, ok: true},
		{in: "long_context", want: TierLongContext, ok: true},
		{in: "  CHEAP  ", want: TierCheap, ok: true},
		{in: "gpt-4.1", ok: false},
	}
	for _, tt := range tests {
		got, err := ParseCapabilityTier(tt.in)
		if tt.ok && err != nil {
			t.Fatalf("ParseCapabilityTier(%q) unexpected error: %v", tt.in, err)
		}
		if !tt.ok && err == nil {
			t.Fatalf("ParseCapabilityTier(%q) expected error", tt.in)
		}
		if tt.ok && got != tt.want {
			t.Fatalf("ParseCapabilityTier(%q)=%q want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveModelForTier_UsesProviderFixtureMapping(t *testing.T) {
	adapter := fixtureProviderAdapter{
		name: "fixture-runtime",
		models: map[CapabilityTier][]ModelSpec{
			TierCheap: {
				{ID: "provider-lite-v1", MaxContextTokens: 8000},
			},
			TierStrong: {
				{ID: "provider-pro-v2", MaxContextTokens: 32000, SupportsTools: true},
			},
		},
	}
	got, err := ResolveModelForTier(adapter, TierStrong)
	if err != nil {
		t.Fatalf("ResolveModelForTier failed: %v", err)
	}
	if got.ID != "provider-pro-v2" {
		t.Fatalf("expected strong-tier fixture model, got %q", got.ID)
	}
}

func TestSelectCapabilityTier(t *testing.T) {
	if got := SelectCapabilityTier(1000, "low", 0); got != TierCheap {
		t.Fatalf("expected cheap tier, got %s", got)
	}
	if got := SelectCapabilityTier(12000, "low", 0); got != TierBalanced {
		t.Fatalf("expected balanced tier, got %s", got)
	}
	if got := SelectCapabilityTier(20000, "high", 0); got != TierStrong {
		t.Fatalf("expected strong tier for high risk, got %s", got)
	}
	if got := SelectCapabilityTier(70000, "low", 0); got != TierLongContext {
		t.Fatalf("expected long_context tier, got %s", got)
	}
}
