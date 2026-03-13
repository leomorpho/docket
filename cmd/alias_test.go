package cmd

import "testing"

func TestCommandAliases(t *testing.T) {
	tests := []struct {
		alias    string
		expected string
	}{
		{alias: "ls", expected: "list"},
		{alias: "st", expected: "status"},
		{alias: "bd", expected: "board"},
	}

	for _, tt := range tests {
		cmd, _, err := rootCmd.Find([]string{tt.alias})
		if err != nil {
			t.Fatalf("failed to find alias %q: %v", tt.alias, err)
		}
		if cmd == nil {
			t.Fatalf("command for alias %q was nil", tt.alias)
		}
		if cmd.Use != tt.expected {
			t.Errorf("alias %q resolved to command %q, want %q", tt.alias, cmd.Use, tt.expected)
		}
	}
}
