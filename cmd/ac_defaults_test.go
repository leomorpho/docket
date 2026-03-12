package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectStackFromRootManifests(t *testing.T) {
	cases := []struct {
		name     string
		file     string
		content  string
		expected string
	}{
		{name: "python", file: "pyproject.toml", content: "[project]\nname='x'\n", expected: "python"},
		{name: "go", file: "go.mod", content: "module x\n", expected: "go"},
		{name: "rust", file: "Cargo.toml", content: "[package]\nname='x'\n", expected: "rust"},
		{name: "javascript", file: "package.json", content: `{"name":"x"}`, expected: "javascript"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			if err := os.WriteFile(filepath.Join(tmp, tc.file), []byte(tc.content), 0o644); err != nil {
				t.Fatalf("write file failed: %v", err)
			}
			if got := detectStack(tmp); got != tc.expected {
				t.Fatalf("detectStack=%q want %q", got, tc.expected)
			}
		})
	}
}
