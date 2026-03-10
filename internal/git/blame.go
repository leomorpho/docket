package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// BlameResult holds the result of blaming a single line.
type BlameResult struct {
	SHA     string
	Author  string
	Date    string
	Summary string
}

// BlameLine runs git blame on a single line and returns commit info.
func BlameLine(repoRoot, file string, line int) (*BlameResult, error) {
	if line <= 0 {
		return nil, fmt.Errorf("line must be > 0")
	}

	out, err := runGit(repoRoot, "blame", "-L", fmt.Sprintf("%d,%d", line, line), "--porcelain", file)
	if err != nil {
		return nil, fmt.Errorf("running git blame: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return nil, fmt.Errorf("unexpected empty blame output")
	}

	first := strings.Fields(lines[0])
	if len(first) == 0 {
		return nil, fmt.Errorf("failed to parse blame commit SHA")
	}

	res := &BlameResult{SHA: first[0]}
	var authorTime int64

	for _, l := range lines[1:] {
		switch {
		case strings.HasPrefix(l, "author "):
			res.Author = strings.TrimSpace(strings.TrimPrefix(l, "author "))
		case strings.HasPrefix(l, "author-time "):
			ts := strings.TrimSpace(strings.TrimPrefix(l, "author-time "))
			v, parseErr := strconv.ParseInt(ts, 10, 64)
			if parseErr == nil {
				authorTime = v
			}
		case strings.HasPrefix(l, "summary "):
			res.Summary = strings.TrimSpace(strings.TrimPrefix(l, "summary "))
		}
	}

	if authorTime > 0 {
		res.Date = time.Unix(authorTime, 0).UTC().Format("2006-01-02")
	}

	return res, nil
}

// CommitTicket returns the ticket ID from a commit's Ticket: trailer, or "" if none.
func CommitTicket(repoRoot, sha string) (string, error) {
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return "", fmt.Errorf("sha is required")
	}

	out, err := runGit(repoRoot, "log", "-1", "--format=%(trailers:key=Ticket,valueonly)", sha)
	if err == nil {
		if v := strings.TrimSpace(out); v != "" {
			return v, nil
		}
		return "", nil
	}

	// Graceful fallback for older git versions: inspect commit body directly.
	body, bodyErr := runGit(repoRoot, "show", "-s", "--format=%B", sha)
	if bodyErr != nil {
		return "", fmt.Errorf("reading commit trailers: %w", err)
	}

	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Ticket:") {
			id := strings.TrimSpace(strings.TrimPrefix(trimmed, "Ticket:"))
			if id != "" {
				return id, nil
			}
		}
	}

	return "", nil
}

func runGit(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}

	return stdout.String(), nil
}
