package adapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	ErrUnsupportedAdapter = errors.New("unsupported adapter")
	ErrIncompleteAdapter  = errors.New("adapter surface is not implemented")
)

type Metadata struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Surfaces    []string `json:"surfaces"`
}

type BootstrapInput struct {
	RepoRoot string `json:"repo_root,omitempty"`
}

type InstallInput struct {
	RepoRoot string `json:"repo_root,omitempty"`
}

type DoctorReport struct {
	Checks []DoctorCheck `json:"checks,omitempty"`
}

type DoctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type StatusReport struct {
	Ready   bool   `json:"ready"`
	Summary string `json:"summary,omitempty"`
}

type Adapter interface {
	Metadata() Metadata
	Detect(repoRoot string) bool
	Bootstrap(ctx context.Context, input BootstrapInput) error
	Doctor(ctx context.Context, repoRoot string) (DoctorReport, error)
	Status(ctx context.Context, repoRoot string) (StatusReport, error)
	Install(ctx context.Context, input InstallInput) error
}

type Registry struct {
	orderedIDs []string
	adapters   map[string]Adapter
}

func NewRegistry(adapters ...Adapter) *Registry {
	r := &Registry{
		adapters: make(map[string]Adapter, len(adapters)),
	}
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		id := strings.TrimSpace(adapter.Metadata().ID)
		if id == "" {
			continue
		}
		id = strings.ToLower(id)
		if _, exists := r.adapters[id]; exists {
			continue
		}
		r.adapters[id] = adapter
		r.orderedIDs = append(r.orderedIDs, id)
	}
	return r
}

func DefaultRegistry() *Registry {
	return NewRegistry(
		newStubAdapter("codex", "Codex", "AGENTS.md"),
		newStubAdapter("claude-code", "Claude Code", "CLAUDE.md"),
		newStubAdapter("gemini", "Gemini CLI", "GEMINI.md"),
	)
}

func (r *Registry) Resolve(repoRoot, requestedID string) Adapter {
	if r == nil {
		return newUnsupportedAdapter("unknown", nil)
	}
	if req := strings.TrimSpace(strings.ToLower(requestedID)); req != "" {
		if adapter, ok := r.adapters[req]; ok {
			return adapter
		}
		return newUnsupportedAdapter(req, r.AvailableIDs())
	}
	for _, id := range r.orderedIDs {
		adapter := r.adapters[id]
		if adapter != nil && adapter.Detect(repoRoot) {
			return adapter
		}
	}
	return newUnsupportedAdapter("auto-detect", r.AvailableIDs())
}

func (r *Registry) AvailableIDs() []string {
	if r == nil {
		return nil
	}
	ids := append([]string(nil), r.orderedIDs...)
	sort.Strings(ids)
	return ids
}

func RunBootstrap(ctx context.Context, adapter Adapter, input BootstrapInput) error {
	if adapter == nil {
		return fmt.Errorf("%w: adapter is required", ErrUnsupportedAdapter)
	}
	return adapter.Bootstrap(ctx, input)
}

func RunDoctor(ctx context.Context, adapter Adapter, repoRoot string) (DoctorReport, error) {
	if adapter == nil {
		return DoctorReport{}, fmt.Errorf("%w: adapter is required", ErrUnsupportedAdapter)
	}
	return adapter.Doctor(ctx, repoRoot)
}

func RunStatus(ctx context.Context, adapter Adapter, repoRoot string) (StatusReport, error) {
	if adapter == nil {
		return StatusReport{}, fmt.Errorf("%w: adapter is required", ErrUnsupportedAdapter)
	}
	return adapter.Status(ctx, repoRoot)
}

func RunInstall(ctx context.Context, adapter Adapter, input InstallInput) error {
	if adapter == nil {
		return fmt.Errorf("%w: adapter is required", ErrUnsupportedAdapter)
	}
	return adapter.Install(ctx, input)
}

type stubAdapter struct {
	metadata    Metadata
	detectFiles []string
}

func newStubAdapter(id, name string, detectFiles ...string) Adapter {
	return stubAdapter{
		metadata: Metadata{
			ID:          id,
			DisplayName: name,
			Surfaces:    []string{"bootstrap", "doctor", "status", "install"},
		},
		detectFiles: detectFiles,
	}
}

func (s stubAdapter) Metadata() Metadata {
	return s.metadata
}

func (s stubAdapter) Detect(repoRoot string) bool {
	for _, marker := range s.detectFiles {
		if marker == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(repoRoot, marker)); err == nil {
			return true
		}
	}
	return false
}

func (s stubAdapter) Bootstrap(_ context.Context, _ BootstrapInput) error {
	return surfaceNotImplementedError(s.metadata.ID, "bootstrap")
}

func (s stubAdapter) Doctor(_ context.Context, _ string) (DoctorReport, error) {
	return DoctorReport{}, surfaceNotImplementedError(s.metadata.ID, "doctor")
}

func (s stubAdapter) Status(_ context.Context, _ string) (StatusReport, error) {
	return StatusReport{}, surfaceNotImplementedError(s.metadata.ID, "status")
}

func (s stubAdapter) Install(_ context.Context, _ InstallInput) error {
	return surfaceNotImplementedError(s.metadata.ID, "install")
}

type unsupportedAdapter struct {
	requested string
	available []string
}

func newUnsupportedAdapter(requested string, available []string) Adapter {
	return unsupportedAdapter{requested: requested, available: append([]string(nil), available...)}
}

func (u unsupportedAdapter) Metadata() Metadata {
	return Metadata{
		ID:          "unsupported",
		DisplayName: "Unsupported Adapter",
		Surfaces:    nil,
	}
}

func (u unsupportedAdapter) Detect(_ string) bool {
	return false
}

func (u unsupportedAdapter) Bootstrap(_ context.Context, _ BootstrapInput) error {
	return unsupportedError(u.requested, u.available)
}

func (u unsupportedAdapter) Doctor(_ context.Context, _ string) (DoctorReport, error) {
	return DoctorReport{}, unsupportedError(u.requested, u.available)
}

func (u unsupportedAdapter) Status(_ context.Context, _ string) (StatusReport, error) {
	return StatusReport{}, unsupportedError(u.requested, u.available)
}

func (u unsupportedAdapter) Install(_ context.Context, _ InstallInput) error {
	return unsupportedError(u.requested, u.available)
}

func unsupportedError(requested string, available []string) error {
	if len(available) == 0 {
		return fmt.Errorf("%w: %q (no adapters registered)", ErrUnsupportedAdapter, requested)
	}
	return fmt.Errorf("%w: %q (available: %s)", ErrUnsupportedAdapter, requested, strings.Join(available, ", "))
}

func surfaceNotImplementedError(adapterID, surface string) error {
	return fmt.Errorf("%w: adapter %q does not implement %q", ErrIncompleteAdapter, adapterID, surface)
}
