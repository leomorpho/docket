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
		repoAbs, _ := filepath.Abs(ctx.Repo)
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

	m.Register(Registration{
		Name:  "enforce-privileged-authorization",
		Event: EventPrivileged,
		Mode:  ModeEnforcement,
		Run: func(ctx Context) error {
			if !ctx.PrivilegedAuthorized {
				return fmt.Errorf("privileged transition requires secure-mode authorization")
			}
			return nil
		},
	})
}
