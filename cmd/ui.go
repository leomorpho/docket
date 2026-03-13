package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/leomorpho/docket/internal/git"
	"github.com/spf13/cobra"
)

var (
	uiOpen   bool
	uiNoOpen bool
)

const uiHubPort = 43173

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch the local Svelte UI dev server",
	Long:  "Installs UI dependencies, starts the SvelteKit dev server, prints the local URL, and can open it in your default browser. If a docket UI server is already running it registers this project with it instead of starting a new one.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if uiOpen && uiNoOpen {
			return fmt.Errorf("--open and --no-open cannot be used together")
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		hubURL := fmt.Sprintf("http://127.0.0.1:%d", uiHubPort)

		// If a docket UI server is already running, register this project and open.
		if projectID, ok := registerWithHub(hubURL, cwd); ok {
			openURL := fmt.Sprintf("%s/?project=%s", hubURL, projectID)
			fmt.Fprintf(cmd.OutOrStdout(), "Docket UI already running — registered %q\n", filepath.Base(cwd))
			fmt.Fprintf(cmd.OutOrStdout(), "Open: %s\n", openURL)
			shouldOpen := uiOpen
			if !uiOpen && !uiNoOpen && isInteractiveStdin() {
				shouldOpen = promptOpen(cmd, openURL)
			}
			if shouldOpen {
				if err := openBrowser(openURL); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to open browser: %v\n", err)
				}
			}
			return nil
		}

		webDir, err := resolveWebDir()
		if err != nil {
			return err
		}
		if _, err := exec.LookPath("pnpm"); err != nil {
			return fmt.Errorf("pnpm not found in PATH. Install pnpm, then retry `docket ui`")
		}

		// Always ensure dependencies are installed so users can run `docket ui` directly.
		fmt.Fprintln(cmd.OutOrStdout(), "Ensuring UI dependencies are installed...")
		install := exec.Command("pnpm", "install")
		install.Dir = webDir
		install.Stdout = cmd.OutOrStdout()
		install.Stderr = cmd.ErrOrStderr()
		install.Stdin = os.Stdin
		if err := install.Run(); err != nil {
			return fmt.Errorf("failed to install UI dependencies: %w", err)
		}

		fallbackURL := fmt.Sprintf("http://127.0.0.1:%d/", uiHubPort)
		fmt.Fprintf(cmd.OutOrStdout(), "Starting Docket UI on %s\n", fallbackURL)

		shouldOpen := uiOpen
		if !uiOpen && !uiNoOpen && isInteractiveStdin() {
			shouldOpen = promptOpen(cmd, fallbackURL)
		}

		proc := exec.Command("pnpm", "dev", "--host", "127.0.0.1", "--port", strconv.Itoa(uiHubPort), "--strictPort")
		proc.Dir = webDir
		proc.Env = append(os.Environ(), "DOCKET_DIR="+cwd)
		proc.Stdin = os.Stdin
		stdout, err := proc.StdoutPipe()
		if err != nil {
			return err
		}
		stderr, err := proc.StderrPipe()
		if err != nil {
			return err
		}
		if err := proc.Start(); err != nil {
			return err
		}

		localURLCh := make(chan string, 1)
		var wg sync.WaitGroup
		wg.Add(2)
		go streamPipe(stdout, cmd.OutOrStdout(), true, localURLCh, &wg)
		go streamPipe(stderr, cmd.ErrOrStderr(), false, localURLCh, &wg)

		if shouldOpen {
			go func() {
				serverURL := fallbackURL
				select {
				case detected := <-localURLCh:
					serverURL = strings.TrimSuffix(detected, "/")
				case <-time.After(6 * time.Second):
				}
				// Register and open with project param.
				if projectID, ok := registerWithHubRetry(serverURL, cwd, 10*time.Second); ok {
					serverURL = fmt.Sprintf("%s/?project=%s", serverURL, projectID)
				}
				if err := openBrowser(serverURL); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to open browser: %v\n", err)
				}
			}()
		}

		err = proc.Wait()
		wg.Wait()
		return err
	},
}

// registerWithHub attempts to register the project dir with an existing docket UI
// server. Returns the project ID and true on success.
func registerWithHub(hubURL, dir string) (string, bool) {
	// First check the server is a docket UI instance.
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(hubURL + "/api/health")
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	var health struct {
		OK      bool   `json:"ok"`
		Service string `json:"service"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil || health.Service != "docket-ui" {
		return "", false
	}

	return postRegister(hubURL, dir, 2*time.Second)
}

// registerWithHubRetry retries registration until the server is ready or timeout.
func registerWithHubRetry(hubURL, dir string, timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if id, ok := postRegister(hubURL, dir, time.Second); ok {
			return id, true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return "", false
}

func postRegister(hubURL, dir string, timeout time.Duration) (string, bool) {
	payload, _ := json.Marshal(map[string]string{"dir": dir})
	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(hubURL+"/api/projects/register", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	var result struct {
		OK      bool `json:"ok"`
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || !result.OK {
		return "", false
	}
	return result.Project.ID, true
}

func resolveWebDir() (string, error) {
	if env := os.Getenv("DOCKET_UI_DIR"); env != "" {
		if hasWebPackage(env) {
			return env, nil
		}
		return "", fmt.Errorf("DOCKET_UI_DIR=%q does not contain web/package.json (docket-ui)", env)
	}

	// 1. Try from the binary executable location (for installed binaries)
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			if p := walkForWeb(filepath.Dir(resolved)); p != "" {
				return p, nil
			}
		}
	}

	// 2. Try from the Git root of the repo being run (if it's the docket repo)
	if root, err := git.GetRepoRoot(repo); err == nil {
		if p := walkForWeb(root); p != "" {
			return p, nil
		}
	}

	// 3. Try using runtime.Caller (works if built from source on this machine)
	if _, filename, _, ok := runtime.Caller(0); ok {
		if p := walkForWeb(filepath.Dir(filename)); p != "" {
			return p, nil
		}
	}

	// 4. Fallback to walking up from current working directory
	if cwd, err := os.Getwd(); err == nil {
		if p := walkForWeb(cwd); p != "" {
			return p, nil
		}
	}

	return "", fmt.Errorf("web UI directory not found. Please set DOCKET_UI_DIR to the web/ path of the Docket source repo")
}

func walkForWeb(start string) string {
	cur := start
	for i := 0; i < 12; i++ {
		candidate := filepath.Join(cur, "web")
		if hasWebPackage(candidate) {
			return candidate
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return ""
}

func hasWebPackage(dir string) bool {
	path := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}
	return pkg.Name == "docket-ui"
}

func isInteractiveStdin() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func promptOpen(cmd *cobra.Command, url string) bool {
	fmt.Fprintf(cmd.OutOrStdout(), "Open %s in your default browser? [Y/n]: ", url)
	reader := bufio.NewReader(os.Stdin)
	ans, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	return parseOpenAnswer(ans)
}

func parseOpenAnswer(raw string) bool {
	a := strings.ToLower(strings.TrimSpace(raw))
	if a == "" {
		return true
	}
	return a == "y" || a == "yes"
}

func openBrowser(url string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	return c.Start()
}

var localURLRe = regexp.MustCompile(`https?://[^\s]+`)

func streamPipe(r io.Reader, w io.Writer, detectURL bool, localURLCh chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		fmt.Fprintln(w, line)
		if !detectURL {
			continue
		}
		if strings.Contains(line, "Local:") {
			if u := localURLRe.FindString(line); u != "" {
				select {
				case localURLCh <- u:
				default:
				}
			}
		}
	}
}

func init() {
	uiCmd.Flags().BoolVar(&uiOpen, "open", false, "open the UI URL in your default browser")
	uiCmd.Flags().BoolVar(&uiNoOpen, "no-open", false, "do not prompt to open the UI URL")
	rootCmd.AddCommand(uiCmd)
}
