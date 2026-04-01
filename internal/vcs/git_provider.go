package vcs

import (
	"context"
	"os"
	"path/filepath"

	"github.com/leomorpho/docket/internal/git"
)

type GitProvider struct {
	checkoutPath string
	sharedRoot   string
}

var _ Provider = (*GitProvider)(nil)

func NewGitProvider(repoRoot string) *GitProvider {
	sharedRoot := repoRoot
	if commonDir, err := git.GetGitCommonDir(repoRoot); err == nil && commonDir != "" {
		sharedRoot = filepath.Dir(commonDir)
	} else if topLevel, err := git.GetRepoRoot(repoRoot); err == nil && topLevel != "" {
		sharedRoot = topLevel
	}
	return &GitProvider{
		checkoutPath: repoRoot,
		sharedRoot:   sharedRoot,
	}
}

func (p *GitProvider) CreateWorktree(ctx context.Context, ticketID, branch, path string) error {
	return git.CreateWorktree(p.sharedRoot, ticketID, branch, path)
}

func (p *GitProvider) RemoveWorktree(ctx context.Context, path string) error {
	return git.RemoveWorktree(p.sharedRoot, path)
}

func (p *GitProvider) GetAgentWorktreeDir(ctx context.Context, ticketID string) (string, error) {
	repoRoot, err := filepath.Abs(p.sharedRoot)
	if err != nil {
		return "", err
	}
	return git.GetAgentWorktreeDir(repoRoot, ticketID)
}

func (p *GitProvider) CurrentCheckoutPath(ctx context.Context) (string, error) {
	return filepath.Abs(p.checkoutPath)
}

func (p *GitProvider) IsPrimaryCheckout(ctx context.Context) (bool, error) {
	info, err := os.Stat(filepath.Join(p.checkoutPath, ".git"))
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func (p *GitProvider) GetRepoRoot(ctx context.Context) (string, error) {
	return filepath.Abs(p.sharedRoot)
}

func (p *GitProvider) CommitAll(ctx context.Context, worktreePath, message string) error {
	return git.CommitAll(worktreePath, message)
}

func (p *GitProvider) MergeBranch(ctx context.Context, branch, message string) error {
	return git.MergeBranch(p.sharedRoot, branch, message)
}

func (p *GitProvider) DeleteBranch(ctx context.Context, branch string) error {
	return git.DeleteBranch(p.sharedRoot, branch)
}
