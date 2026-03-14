package workflow

import (
	"fmt"
	"strings"
)

type CapabilityTier string

const (
	TierCheap       CapabilityTier = "cheap"
	TierBalanced    CapabilityTier = "balanced"
	TierStrong      CapabilityTier = "strong"
	TierLongContext CapabilityTier = "long_context"
)

type ModelSpec struct {
	ID               string
	MaxContextTokens int
	SupportsTools    bool
}

type ProviderAdapter interface {
	ProviderName() string
	ModelsByTier() map[CapabilityTier][]ModelSpec
}

type StaticProviderAdapter struct {
	Name   string
	Models map[CapabilityTier][]ModelSpec
}

func (a StaticProviderAdapter) ProviderName() string {
	return a.Name
}

func (a StaticProviderAdapter) ModelsByTier() map[CapabilityTier][]ModelSpec {
	return a.Models
}

type RoutingDecision struct {
	PreferredTier        CapabilityTier
	SelectedTier         CapabilityTier
	Rationale            string
	PreferredUnavailable bool
}

func DefaultProviderAdapter() ProviderAdapter {
	return StaticProviderAdapter{
		Name: "default-runtime",
		Models: map[CapabilityTier][]ModelSpec{
			TierCheap: {
				{ID: "default-cheap", MaxContextTokens: 8000},
			},
			TierBalanced: {
				{ID: "default-balanced", MaxContextTokens: 32000, SupportsTools: true},
			},
			TierStrong: {
				{ID: "default-strong", MaxContextTokens: 64000, SupportsTools: true},
			},
			TierLongContext: {
				{ID: "default-long-context", MaxContextTokens: 128000, SupportsTools: true},
			},
		},
	}
}

func ParseCapabilityTier(raw string) (CapabilityTier, error) {
	tier := CapabilityTier(strings.ToLower(strings.TrimSpace(raw)))
	switch tier {
	case TierCheap, TierBalanced, TierStrong, TierLongContext:
		return tier, nil
	default:
		return "", fmt.Errorf("invalid capability tier %q", raw)
	}
}

func SelectCapabilityTier(tokenEstimate int, risk string, failureCount int) CapabilityTier {
	if tokenEstimate > 64000 {
		return TierLongContext
	}
	if strings.EqualFold(strings.TrimSpace(risk), "high") || failureCount >= 2 {
		return TierStrong
	}
	if tokenEstimate <= 4000 && failureCount == 0 {
		return TierCheap
	}
	return TierBalanced
}

func ResolveModelForTier(adapter ProviderAdapter, tier CapabilityTier) (ModelSpec, error) {
	if _, err := ParseCapabilityTier(string(tier)); err != nil {
		return ModelSpec{}, err
	}
	if adapter == nil {
		return ModelSpec{}, fmt.Errorf("provider adapter is required")
	}
	models := adapter.ModelsByTier()[tier]
	if len(models) == 0 {
		return ModelSpec{}, fmt.Errorf("provider %q does not expose models for tier %q", adapter.ProviderName(), tier)
	}
	return models[0], nil
}

func ResolveModelForTask(adapter ProviderAdapter, preferred CapabilityTier) (ModelSpec, RoutingDecision, error) {
	if _, err := ParseCapabilityTier(string(preferred)); err != nil {
		return ModelSpec{}, RoutingDecision{}, err
	}
	if adapter == nil {
		return ModelSpec{}, RoutingDecision{}, fmt.Errorf("provider adapter is required")
	}
	catalog := adapter.ModelsByTier()
	for idx, tier := range fallbackOrder(preferred) {
		if models := catalog[tier]; len(models) > 0 {
			decision := RoutingDecision{
				PreferredTier:        preferred,
				SelectedTier:         tier,
				PreferredUnavailable: idx > 0,
			}
			if idx == 0 {
				decision.Rationale = fmt.Sprintf("selected preferred tier %s from deterministic routing profile", preferred)
			} else {
				decision.Rationale = fmt.Sprintf("preferred tier %s unavailable, fell back to %s", preferred, tier)
			}
			return models[0], decision, nil
		}
	}
	return ModelSpec{}, RoutingDecision{}, fmt.Errorf("provider %q has no models for fallback chain from tier %q", adapter.ProviderName(), preferred)
}

func fallbackOrder(preferred CapabilityTier) []CapabilityTier {
	switch preferred {
	case TierCheap:
		return []CapabilityTier{TierCheap, TierBalanced, TierStrong}
	case TierBalanced:
		return []CapabilityTier{TierBalanced, TierStrong, TierCheap}
	case TierStrong:
		return []CapabilityTier{TierStrong, TierBalanced, TierCheap}
	case TierLongContext:
		return []CapabilityTier{TierLongContext, TierStrong, TierBalanced}
	default:
		return []CapabilityTier{TierBalanced, TierStrong, TierCheap}
	}
}
