package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

var docketHome string

func ensureDocketHome() error {
	if docketHome != "" {
		return nil
	}
	home, err := resolveDocketHome()
	if err != nil {
		return err
	}
	docketHome = home
	return nil
}

func resolveDocketHome() (string, error) {
	env := os.Getenv("DOCKET_HOME")
	if env == "" {
		example := filepath.Join(os.TempDir(), "docket-home")
		return "", fmt.Errorf("DOCKET_HOME is required to keep Docket's secure state outside checked-in repositories. Example: DOCKET_HOME=%s", example)
	}

	abs, err := filepath.Abs(env)
	if err != nil {
		return "", fmt.Errorf("DOCKET_HOME=%q could not be resolved: %w", env, err)
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return "", fmt.Errorf("DOCKET_HOME=%q is not writable: %w", abs, err)
	}
	return abs, nil
}
