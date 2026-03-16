package adapters

import (
	"context"
	"encoding/json"
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

type Resolution struct {
	Adapter    Adapter  `json:"-"`
	AdapterID  string   `json:"adapter_id"`
	Source     string   `json:"source"`
	Candidates []string `json:"candidates,omitempty"`
	Warning    string   `json:"warning,omitempty"`
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
	return r.ResolveWithInfo(repoRoot, requestedID).Adapter
}

func (r *Registry) ResolveWithInfo(repoRoot, requestedID string) Resolution {
	if r == nil {
		adapter := newUnsupportedAdapter("unknown", nil)
		return Resolution{Adapter: adapter, AdapterID: adapter.Metadata().ID, Source: "none"}
	}
	if req := strings.TrimSpace(strings.ToLower(requestedID)); req != "" {
		if adapter, ok := r.adapters[req]; ok {
			return Resolution{Adapter: adapter, AdapterID: adapter.Metadata().ID, Source: "override"}
		}
		adapter := newUnsupportedAdapter(req, r.AvailableIDs())
		return Resolution{
			Adapter:   adapter,
			AdapterID: adapter.Metadata().ID,
			Source:    "override",
			Warning:   fmt.Sprintf("adapter override %q is unsupported", req),
		}
	}

	if env := strings.TrimSpace(strings.ToLower(os.Getenv("DOCKET_ADAPTER"))); env != "" {
		if adapter, ok := r.adapters[env]; ok {
			return Resolution{Adapter: adapter, AdapterID: adapter.Metadata().ID, Source: "env"}
		}
		adapter := newUnsupportedAdapter(env, r.AvailableIDs())
		return Resolution{
			Adapter:   adapter,
			AdapterID: adapter.Metadata().ID,
			Source:    "env",
			Warning:   fmt.Sprintf("DOCKET_ADAPTER=%q is unsupported", env),
		}
	}

	if configHint := readAdapterHint(repoRoot); configHint != "" {
		if adapter, ok := r.adapters[configHint]; ok {
			return Resolution{Adapter: adapter, AdapterID: adapter.Metadata().ID, Source: "config"}
		}
		adapter := newUnsupportedAdapter(configHint, r.AvailableIDs())
		return Resolution{
			Adapter:   adapter,
			AdapterID: adapter.Metadata().ID,
			Source:    "config",
			Warning:   fmt.Sprintf("configured adapter %q is unsupported", configHint),
		}
	}

	candidates := make([]string, 0, len(r.orderedIDs))
	for _, id := range r.orderedIDs {
		adapter := r.adapters[id]
		if adapter != nil && adapter.Detect(repoRoot) {
			candidates = append(candidates, id)
		}
	}
	if len(candidates) == 0 {
		adapter := newUnsupportedAdapter("auto-detect", r.AvailableIDs())
		return Resolution{Adapter: adapter, AdapterID: adapter.Metadata().ID, Source: "auto"}
	}
	selectedID := candidates[0]
	selected := r.adapters[selectedID]
	out := Resolution{
		Adapter:    selected,
		AdapterID:  selected.Metadata().ID,
		Source:     "auto",
		Candidates: candidates,
	}
	if len(candidates) > 1 {
		out.Warning = fmt.Sprintf("ambiguous adapter detection (%s); falling back to %q", strings.Join(candidates, ", "), selectedID)
	}
	return out
}

func (r *Registry) AvailableIDs() []string {
	if r == nil {
		return nil
	}
	ids := append([]string(nil), r.orderedIDs...)
	sort.Strings(ids)
	return ids
}

func readAdapterHint(repoRoot string) string {
	if strings.TrimSpace(repoRoot) == "" {
		return ""
	}
	adapterPath := filepath.Join(repoRoot, ".docket", "adapter")
	if raw, err := os.ReadFile(adapterPath); err == nil {
		if hint := strings.TrimSpace(strings.ToLower(string(raw))); hint != "" {
			return hint
		}
	}
	cfgPath := filepath.Join(repoRoot, ".docket", "config.json")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	hint, _ := payload["adapter"].(string)
	return strings.TrimSpace(strings.ToLower(hint))
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
