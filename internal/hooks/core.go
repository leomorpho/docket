package hooks

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	docketgit "github.com/leomorpho/docket/internal/git"
)

func RegisterCoreHooks(m *Manager) {
	m.Register(Registration{
		Name:  "advisory-review-lifecycle",
		Event: EventReviewGate,
		Mode:  ModeAdvisory,
		Run: func(ctx Context) error {
			if ctx.ManagedRun {
				return nil
			}
			return errors.New("review gate running without managed run manifest")
		},
	})

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
	m.Register(Registration{
		Name:  "enforce-managed-worktree-review",
		Event: EventReviewGate,
		Mode:  ModeEnforcement,
		Run:   enforceWorktree,
	})

	m.Register(Registration{
		Name:  "enforce-commit-linkage-review",
		Event: EventReviewGate,
		Mode:  ModeEnforcement,
		Run: func(ctx Context) error {
			if !ctx.ManagedRun {
				return nil
			}
			ref := strings.TrimSpace(ctx.Branch)
			if ref == "" {
				ref = "HEAD"
			}
			ok, err := docketgit.HasTicketTrailerSince(ctx.Repo, ref, ctx.TicketID, ctx.RunStartedAt)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("no commit on %s references Ticket: %s", ref, ctx.TicketID)
			}
			return nil
		},
	})
}
