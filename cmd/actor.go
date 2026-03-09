package cmd

import (
	"os"
	"os/exec"
	"strings"
)

func detectActor() string {
	if actor := os.Getenv("DOCKET_ACTOR"); actor != "" {
		return actor
	}

	// Try git config user.name
	out, err := exec.Command("git", "config", "user.name").Output()
	if err == nil {
		name := strings.TrimSpace(string(out))
		if name != "" {
			return "human:" + name
		}
	}

	return "human:unknown"
}
