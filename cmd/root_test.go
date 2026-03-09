package cmd

import (
	"bytes"
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

func TestExecuteHelp(t *testing.T) {
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("rootCmd.Execute() failed: %v", err)
	}

	if !bytes.Contains(b.Bytes(), []byte("git-native ticket system")) {
		t.Errorf("expected help message to contain 'git-native ticket system', but got:\n%s", b.String())
	}
}
