package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

var (
	uiOpen   bool
	uiNoOpen bool
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch the local Svelte UI dev server",
	Long:  "Installs UI dependencies, starts the SvelteKit dev server, prints the local URL, and can open it in your default browser.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if uiOpen && uiNoOpen {
			return fmt.Errorf("--open and --no-open cannot be used together")
		}

		webDir, err := resolveWebDir()
		if err != nil {
			return err
		}
		if _, err := exec.LookPath("pnpm"); err != nil {
			return fmt.Errorf("pnpm not found in PATH. Install pnpm, then retry `docket ui`")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
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

		// Use an uncommon high port range to avoid collisions with typical dev servers.
		port, err := pickOpenPort(43173, 200)
		if err != nil {
			return fmt.Errorf("failed to pick UI port: %w", err)
		}
		fallbackURL := fmt.Sprintf("http://127.0.0.1:%d/", port)
		fmt.Fprintf(cmd.OutOrStdout(), "Starting Docket UI on preferred URL: %s\n", fallbackURL)

		shouldOpen := uiOpen
		if !uiOpen && !uiNoOpen && isInteractiveStdin() {
			shouldOpen = promptOpen(cmd, fallbackURL)
		}

		proc := exec.Command("pnpm", "dev", "--host", "127.0.0.1", "--port", strconv.Itoa(port), "--strictPort")
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
				url := fallbackURL
				select {
				case detected := <-localURLCh:
					url = detected
				case <-time.After(6 * time.Second):
				}
				if err := openBrowser(url); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to open browser: %v\n", err)
				}
			}()
		}

		err = proc.Wait()
		wg.Wait()
		return err
	},
}

func resolveWebDir() (string, error) {
	if env := os.Getenv("DOCKET_UI_DIR"); env != "" {
		if hasWebPackage(env) {
			return env, nil
		}
		return "", fmt.Errorf("DOCKET_UI_DIR=%q does not contain web/package.json", env)
	}

	if exe, err := os.Executable(); err == nil {
		if p := walkForWeb(filepath.Dir(exe)); p != "" {
			return p, nil
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if p := walkForWeb(cwd); p != "" {
			return p, nil
		}
	}

	return "", fmt.Errorf("web UI directory not found. Set DOCKET_UI_DIR to the web/ path")
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
	info, err := os.Stat(filepath.Join(dir, "package.json"))
	return err == nil && !info.IsDir()
}

func pickOpenPort(start, attempts int) (int, error) {
	for i := 0; i < attempts; i++ {
		p := start + i
		ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return p, nil
	}
	// Fallback to any available ephemeral port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("no open port found from %d to %d and failed ephemeral fallback: %w", start, start+attempts-1, err)
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("failed to resolve ephemeral TCP port")
	}
	return addr.Port, nil
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
