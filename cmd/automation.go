package cmd

import (
	"os"
	"strings"
)

var automationMode bool

func isAutomationMode() bool {
	if automationMode {
		return true
	}
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("DOCKET_AUTOMATION")))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
