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
