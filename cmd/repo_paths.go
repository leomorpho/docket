package cmd

import (
	"strings"

	docketgit "github.com/leomorpho/docket/internal/git"
)

func ticketRepoRoot(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	return docketgit.SharedRepoRoot(path)
}
