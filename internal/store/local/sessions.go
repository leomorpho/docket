package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type SessionFile struct {
	Name       string
	Path       string
	SizeBytes  int64
	ModTimeUTC time.Time
}

func (s *Store) sessionsDir(id string) string {
	return filepath.Join(s.RepoRoot, ".docket", "tickets", id, "sessions")
}

func (s *Store) AttachSession(ctx context.Context, id, sourcePath string) (string, error) {
	t, err := s.GetTicket(ctx, id)
	if err != nil {
		return "", err
	}
	if t == nil {
		return "", fmt.Errorf("ticket %s not found", id)
	}

	in, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer in.Close()

	if err := os.MkdirAll(s.sessionsDir(id), 0755); err != nil {
		return "", err
	}

	name := time.Now().UTC().Format("2006-01-02T150405Z") + filepath.Ext(sourcePath)
	if filepath.Ext(sourcePath) == "" {
		name += ".log"
	}
	dstPath := filepath.Join(s.sessionsDir(id), name)

	out, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}

	return filepath.ToSlash(filepath.Join("sessions", name)), nil
}

func (s *Store) ListSessions(ctx context.Context, id string) ([]SessionFile, error) {
	t, err := s.GetTicket(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("ticket %s not found", id)
	}

	entries, err := os.ReadDir(s.sessionsDir(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	out := make([]SessionFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, SessionFile{
			Name:       e.Name(),
			Path:       filepath.Join(s.sessionsDir(id), e.Name()),
			SizeBytes:  info.Size(),
			ModTimeUTC: info.ModTime().UTC(),
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ModTimeUTC.After(out[j].ModTimeUTC)
	})

	return out, nil
}

func (s *Store) ResolveSessionPath(ctx context.Context, id, name string) (string, error) {
	files, err := s.ListSessions(ctx, id)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no sessions found for %s", id)
	}

	if strings.TrimSpace(name) == "" {
		for _, f := range files {
			if !strings.HasSuffix(f.Name, ".compressed") {
				return f.Path, nil
			}
		}
		return files[0].Path, nil
	}

	for _, f := range files {
		if f.Name == name {
			return f.Path, nil
		}
	}

	return "", fmt.Errorf("session %s not found", name)
}

func (s *Store) MarkSessionCompressed(path string) (string, error) {
	if strings.HasSuffix(path, ".compressed") {
		return path, nil
	}
	newPath := path + ".compressed"
	if err := os.Rename(path, newPath); err != nil {
		return "", err
	}
	return newPath, nil
}
