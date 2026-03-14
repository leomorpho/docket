package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var docketHome string

var (
	docketHomeInteractiveFn = isInteractiveSession
	docketHomePromptFn      = promptDocketHomeBootstrap
)

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
		if fromRC, ok := resolveDocketHomeFromShellRC(); ok {
			_ = os.Setenv("DOCKET_HOME", fromRC)
			env = fromRC
		}
	}
	if env == "" {
		defaultPath, err := defaultDocketHomePath()
		if err != nil {
			return "", err
		}
		if docketHomeInteractiveFn() {
			ok, err := docketHomePromptFn(defaultPath)
			if err != nil {
				return "", err
			}
			if ok {
				if err := os.Setenv("DOCKET_HOME", defaultPath); err != nil {
					return "", fmt.Errorf("failed to set DOCKET_HOME in current process: %w", err)
				}
				env = defaultPath
			}
		}
	}

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

var docketHomeExportRe = regexp.MustCompile(`(?m)^\s*(?:export\s+)?DOCKET_HOME\s*=\s*(.+?)\s*$`)

func resolveDocketHomeFromShellRC() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	for _, rc := range []string{filepath.Join(home, ".zshrc"), filepath.Join(home, ".bashrc")} {
		raw, err := os.ReadFile(rc)
		if err != nil {
			continue
		}
		m := docketHomeExportRe.FindStringSubmatch(string(raw))
		if len(m) < 2 {
			continue
		}
		val := strings.TrimSpace(m[1])
		val = strings.Trim(val, `"'`)
		if val == "" {
			continue
		}
		val = os.Expand(val, func(k string) string { return os.Getenv(k) })
		return val, true
	}
	return "", false
}

func defaultDocketHomePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home directory for DOCKET_HOME default: %w", err)
	}
	return filepath.Join(home, ".docket-home"), nil
}

func isInteractiveSession() bool {
	in, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	out, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (in.Mode()&os.ModeCharDevice) != 0 && (out.Mode()&os.ModeCharDevice) != 0
}

func promptDocketHomeBootstrap(defaultPath string) (bool, error) {
	fmt.Fprintf(os.Stderr, "DOCKET_HOME is not set.\n")
	fmt.Fprintf(os.Stderr, "Set DOCKET_HOME=%s and add it to ~/.zshrc and ~/.bashrc? [Y/n]: ", defaultPath)

	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false, nil
	}
	choice := strings.TrimSpace(strings.ToLower(answer))
	if choice != "" && choice != "y" && choice != "yes" {
		return false, nil
	}

	if err := os.MkdirAll(defaultPath, 0o755); err != nil {
		return false, fmt.Errorf("failed to create DOCKET_HOME default at %s: %w", defaultPath, err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("failed to resolve home directory: %w", err)
	}
	zshrc := filepath.Join(home, ".zshrc")
	bashrc := filepath.Join(home, ".bashrc")
	if _, err := ensureDocketHomeExportInShellRC(zshrc, defaultPath); err != nil {
		return false, err
	}
	if _, err := ensureDocketHomeExportInShellRC(bashrc, defaultPath); err != nil {
		return false, err
	}

	fmt.Fprintf(os.Stderr, "Configured DOCKET_HOME=%s.\n", defaultPath)
	fmt.Fprintf(os.Stderr, "For the current shell, run: export DOCKET_HOME=%q\n", defaultPath)
	return true, nil
}

func ensureDocketHomeExportInShellRC(rcPath, docketHomePath string) (bool, error) {
	data, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to read %s: %w", rcPath, err)
	}

	content := string(data)
	alreadySet := regexp.MustCompile(`(?m)^\s*(export\s+)?DOCKET_HOME=`).MatchString(content)
	if alreadySet {
		return false, nil
	}

	line := fmt.Sprintf("export DOCKET_HOME=%q\n", docketHomePath)
	block := "\n# Added by docket\n" + line

	next := content
	if next != "" && !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	next += block

	if err := os.WriteFile(rcPath, []byte(next), 0o644); err != nil {
		return false, fmt.Errorf("failed to update %s: %w", rcPath, err)
	}
	return true, nil
}
