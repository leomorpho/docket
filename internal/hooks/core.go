package hooks

import (
	"fmt"
	"path/filepath"
	"strings"

	docketgit "github.com/leomorpho/docket/internal/git"
)

func RegisterCoreHooks(m *Manager) {
	enforceWorktree := func(ctx Context) error {
		if !ctx.ManagedRun {
			return nil
		}
		if strings.TrimSpace(ctx.WorktreePath) == "" {
			return fmt.Errorf("managed run requires dedicated worktree path")
		}
		repoRoot := strings.TrimSpace(ctx.Repo)
		if commonDir, err := docketgit.GetGitCommonDir(repoRoot); err == nil && strings.TrimSpace(commonDir) != "" {
			repoRoot = filepath.Dir(commonDir)
		} else if resolvedRoot, err := docketgit.GetRepoRoot(repoRoot); err == nil && strings.TrimSpace(resolvedRoot) != "" {
			repoRoot = resolvedRoot
		}
		repoAbs, _ := filepath.Abs(repoRoot)
		wtAbs, _ := filepath.Abs(ctx.WorktreePath)
		if wtAbs == repoAbs {
			return fmt.Errorf("managed run is bound to main checkout; dedicated worktree is required")
		}
		return nil
	}
	m.Register(Registration{
		Name:  "enforce-managed-worktree-start",
		Event: EventRunStart,
		Mode:  ModeEnforcement,
		Run:   enforceWorktree,
	})
}
