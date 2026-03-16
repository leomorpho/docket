package adapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryResolveByIDAndFallback(t *testing.T) {
	r := DefaultRegistry()

	got := r.Resolve("", "codex")
	if got.Metadata().ID != "codex" {
		t.Fatalf("expected codex adapter, got %q", got.Metadata().ID)
	}

	unknown := r.Resolve("", "unknown")
	if unknown.Metadata().ID != "unsupported" {
		t.Fatalf("expected unsupported adapter, got %q", unknown.Metadata().ID)
	}
	if err := RunBootstrap(context.Background(), unknown, BootstrapInput{}); !errors.Is(err, ErrUnsupportedAdapter) {
		t.Fatalf("expected ErrUnsupportedAdapter, got: %v", err)
	}
}

func TestRegistryResolveAutoDetectDeterministicOrder(t *testing.T) {
	r := DefaultRegistry()
	repo := t.TempDir()

	if err := os.WriteFile(filepath.Join(repo, "CLAUDE.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "AGENTS.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	got := r.Resolve(repo, "")
	if got.Metadata().ID != "codex" {
		t.Fatalf("expected codex first in deterministic order, got %q", got.Metadata().ID)
	}
}

func TestInterfaceErrorPropagation(t *testing.T) {
	fail := errAdapter{
		metadata:     Metadata{ID: "failing"},
		bootstrapErr: errors.New("bootstrap boom"),
		doctorErr:    errors.New("doctor boom"),
		statusErr:    errors.New("status boom"),
		installErr:   errors.New("install boom"),
	}

	if err := RunBootstrap(context.Background(), fail, BootstrapInput{}); err == nil || !strings.Contains(err.Error(), "bootstrap boom") {
		t.Fatalf("expected bootstrap error to propagate, got %v", err)
	}
	if _, err := RunDoctor(context.Background(), fail, "."); err == nil || !strings.Contains(err.Error(), "doctor boom") {
		t.Fatalf("expected doctor error to propagate, got %v", err)
	}
	if _, err := RunStatus(context.Background(), fail, "."); err == nil || !strings.Contains(err.Error(), "status boom") {
		t.Fatalf("expected status error to propagate, got %v", err)
	}
	if err := RunInstall(context.Background(), fail, InstallInput{}); err == nil || !strings.Contains(err.Error(), "install boom") {
		t.Fatalf("expected install error to propagate, got %v", err)
	}
}

func TestIntegrationAdapterResolutionMatrix(t *testing.T) {
	r := DefaultRegistry()

	cases := []struct {
		name     string
		markers  []string
		expected string
		override string
	}{
		{name: "codex marker", markers: []string{"AGENTS.md"}, expected: "codex"},
		{name: "claude marker", markers: []string{"CLAUDE.md"}, expected: "claude-code"},
		{name: "gemini marker", markers: []string{"GEMINI.md"}, expected: "gemini"},
		{name: "override wins", markers: []string{"AGENTS.md"}, override: "gemini", expected: "gemini"},
	}

	var rows []string
	for _, tc := range cases {
		repo := t.TempDir()
		for _, marker := range tc.markers {
			if err := os.WriteFile(filepath.Join(repo, marker), []byte("marker"), 0o644); err != nil {
				t.Fatalf("write %s: %v", marker, err)
			}
		}
		got := r.Resolve(repo, tc.override).Metadata().ID
		if got != tc.expected {
			t.Fatalf("%s: expected %q got %q", tc.name, tc.expected, got)
		}
		rows = append(rows, fmt.Sprintf("%s -> %s", tc.name, got))
	}
	t.Logf("adapter resolution matrix:\n%s", strings.Join(rows, "\n"))
}

func TestRunFunctionsRejectNilAdapter(t *testing.T) {
	if err := RunBootstrap(context.Background(), nil, BootstrapInput{}); !errors.Is(err, ErrUnsupportedAdapter) {
		t.Fatalf("expected ErrUnsupportedAdapter from RunBootstrap, got: %v", err)
	}
	if _, err := RunDoctor(context.Background(), nil, "."); !errors.Is(err, ErrUnsupportedAdapter) {
		t.Fatalf("expected ErrUnsupportedAdapter from RunDoctor, got: %v", err)
	}
	if _, err := RunStatus(context.Background(), nil, "."); !errors.Is(err, ErrUnsupportedAdapter) {
		t.Fatalf("expected ErrUnsupportedAdapter from RunStatus, got: %v", err)
	}
	if err := RunInstall(context.Background(), nil, InstallInput{}); !errors.Is(err, ErrUnsupportedAdapter) {
		t.Fatalf("expected ErrUnsupportedAdapter from RunInstall, got: %v", err)
	}
}

func TestUnsupportedAdapterErrorIncludesAvailableIDs(t *testing.T) {
	r := DefaultRegistry()
	unsupported := r.Resolve("", "unknown")
	err := RunInstall(context.Background(), unsupported, InstallInput{})
	if err == nil {
		t.Fatal("expected unsupported adapter error")
	}
	msg := err.Error()
	for _, want := range []string{"claude-code", "codex", "gemini"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected unsupported error to include %q, got %q", want, msg)
		}
	}
}

type errAdapter struct {
	metadata     Metadata
	bootstrapErr error
	doctorErr    error
	statusErr    error
	installErr   error
}

func (e errAdapter) Metadata() Metadata                                  { return e.metadata }
func (e errAdapter) Detect(_ string) bool                                { return false }
func (e errAdapter) Bootstrap(_ context.Context, _ BootstrapInput) error { return e.bootstrapErr }
func (e errAdapter) Doctor(_ context.Context, _ string) (DoctorReport, error) {
	return DoctorReport{}, e.doctorErr
}
func (e errAdapter) Status(_ context.Context, _ string) (StatusReport, error) {
	return StatusReport{}, e.statusErr
}
func (e errAdapter) Install(_ context.Context, _ InstallInput) error { return e.installErr }
