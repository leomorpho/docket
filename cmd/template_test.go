package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
)

func TestCreateWithACTemplateAndComposableTemplates(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"create", "--title", "templated", "--desc", "Description long enough for validation constraints in create command.", "--ac-template", "api-endpoint,cli-command"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("create with template failed: %v", err)
	}

	s := local.New(tmp)
	tk, _ := s.GetTicket(context.Background(), "TKT-001")
	if len(tk.AC) < 6 {
		t.Fatalf("expected composable templates to add multiple ACs, got %d", len(tk.AC))
	}
}

func TestTemplateListShowAndUserOverride(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"
	_ = os.MkdirAll(filepath.Join(tmp, ".docket", "templates"), 0o755)
	_ = os.WriteFile(filepath.Join(tmp, ".docket", "templates", "api-endpoint.yaml"), []byte("ac:\n  - desc: \"Custom API AC\"\n    run: \"echo custom\"\n"), 0o644)

	listOut := new(bytes.Buffer)
	rootCmd.SetOut(listOut)
	rootCmd.SetArgs([]string{"template", "list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("template list failed: %v", err)
	}
	if !strings.Contains(listOut.String(), "api-endpoint") {
		t.Fatalf("expected api-endpoint in template list, got: %s", listOut.String())
	}

	showOut := new(bytes.Buffer)
	rootCmd.SetOut(showOut)
	rootCmd.SetArgs([]string{"template", "show", "api-endpoint"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("template show failed: %v", err)
	}
	if !strings.Contains(showOut.String(), "Custom API AC") {
		t.Fatalf("expected user override template content, got: %s", showOut.String())
	}
}
