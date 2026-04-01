package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd(t *testing.T) {
	// Reset flags for test isolation if needed, but here we just check if they exist
	if rootCmd.Use != "docket" {
		t.Errorf("expected Use 'docket', got '%s'", rootCmd.Use)
	}

	formatFlag := rootCmd.PersistentFlags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("expected 'format' persistent flag, but it was not found")
	}
	if formatFlag.DefValue != "human" {
		t.Errorf("expected default format 'human', got '%s'", formatFlag.DefValue)
	}

	repoFlag := rootCmd.PersistentFlags().Lookup("repo")
	if repoFlag == nil {
		t.Fatal("expected 'repo' persistent flag, but it was not found")
	}
}

func assertPrimaryDescriptionLeadsWithNorthStarRuntimeStory(t *testing.T, description string) {
	t.Helper()

	lower := strings.ToLower(description)
	for _, required := range []string{"backlog", "validat", "serial", "autorun"} {
		if !strings.Contains(lower, required) {
			t.Fatalf("primary description should lead with executable backlog runtime semantics and mention %q, got %q", required, description)
		}
	}
	if !strings.Contains(lower, "groom") && !strings.Contains(lower, "runnable") {
		t.Fatalf("primary description should lead with grooming/runnable-work discipline, got %q", description)
	}
	for _, forbidden := range []string{"git-native", "tracker", "security", "review", "parallel"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("primary description should omit historical framing like %q, got %q", forbidden, description)
		}
	}
}

func TestRootHelpSummaryLeadsWithNorthStarRuntimeStory(t *testing.T) {
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("rootCmd.Execute() failed: %v", err)
	}

	assertPrimaryDescriptionLeadsWithNorthStarRuntimeStory(t, rootCmd.Short)

	if !bytes.Contains(b.Bytes(), []byte(rootCmd.Short)) {
		t.Errorf("expected help message to contain updated root summary %q, but got:\n%s", rootCmd.Short, b.String())
	}
}
