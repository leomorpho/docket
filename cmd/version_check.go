package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var Version = "0.1.0"

const versionCheckTTL = 24 * time.Hour

type versionCache struct {
	CheckedAtUnix int64  `json:"checked_at_unix"`
	LatestVersion string `json:"latest_version"`
}

var versionNotice string

var versionCheckWorkerCmd = &cobra.Command{
	Use:    "__version-check-worker",
	Short:  "internal worker for background version check",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cache, _ := loadVersionCache()
		latest, err := fetchLatestReleaseVersion()
		if err != nil || latest == "" {
			return nil
		}
		cache.LatestVersion = latest
		cache.CheckedAtUnix = time.Now().UTC().Unix()
		return saveVersionCache(cache)
	},
}

func init() {
	rootCmd.AddCommand(versionCheckWorkerCmd)
}

func prepareVersionNotice(cmd *cobra.Command) {
	if shouldSkipVersionCheck(cmd) {
		return
	}

	cache, err := loadVersionCache()
	if err == nil {
		if cache.LatestVersion != "" && isVersionNewer(cache.LatestVersion, Version) {
			versionNotice = fmt.Sprintf("update available: %s (current %s). Upgrade: go install github.com/leoaudibert/docket@latest", normalizeVersion(cache.LatestVersion), normalizeVersion(Version))
		}
		if time.Since(time.Unix(cache.CheckedAtUnix, 0)) < versionCheckTTL {
			return
		}
	}
	_ = spawnVersionCheckWorker()
}

func flushVersionNotice(cmd *cobra.Command) {
	if versionNotice == "" || shouldSkipVersionCheck(cmd) {
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "notice: %s\n", versionNotice)
	versionNotice = ""
}

func shouldSkipVersionCheck(cmd *cobra.Command) bool {
	if os.Getenv("DOCKET_DISABLE_VERSION_CHECK") == "1" {
		return true
	}
	if strings.Contains(filepath.Base(os.Args[0]), ".test") {
		return true
	}
	if cmd != nil && cmd.Name() == "__version-check-worker" {
		return true
	}
	return false
}

func spawnVersionCheckWorker() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	c := exec.Command(exe, "__version-check-worker")
	c.Env = os.Environ()
	c.Stdout = nil
	c.Stderr = nil
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}

func fetchLatestReleaseVersion() (string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/leoaudibert/docket/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github release status %d", resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.TagName), nil
}

func versionCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "docket", "version_check.json"), nil
}

func loadVersionCache() (versionCache, error) {
	path, err := versionCachePath()
	if err != nil {
		return versionCache{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return versionCache{}, err
	}
	var cache versionCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return versionCache{}, err
	}
	return cache, nil
}

func saveVersionCache(cache versionCache) error {
	path, err := versionCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

func parseSemverParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	out := make([]int, 3)
	for i := 0; i < len(out) && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return []int{0, 0, 0}
		}
		out[i] = n
	}
	return out
}

func isVersionNewer(latest, current string) bool {
	latestParts := parseSemverParts(latest)
	currentParts := parseSemverParts(current)
	for i := range latestParts {
		if latestParts[i] > currentParts[i] {
			return true
		}
		if latestParts[i] < currentParts[i] {
			return false
		}
	}
	return false
}
