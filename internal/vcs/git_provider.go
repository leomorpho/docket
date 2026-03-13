package vcs

import (
	"context"

	"github.com/leomorpho/docket/internal/git"
)

type GitProvider struct {
	repoRoot string
}

func NewGitProvider(repoRoot string) *GitProvider {
	return &GitProvider{repoRoot: repoRoot}
}

func (p *GitProvider) CreateWorktree(ctx context.Context, ticketID, branch, path string) error {
	return git.CreateWorktree(p.repoRoot, ticketID, branch, path)
}

func (p *GitProvider) RemoveWorktree(ctx context.Context, path string) error {
	return git.RemoveWorktree(p.repoRoot, path)
}

func (p *GitProvider) GetAgentWorktreeDir(ctx context.Context, ticketID string) (string, error) {
	return git.GetAgentWorktreeDir(ticketID)
}

func (p *GitProvider) GetRepoRoot(ctx context.Context) (string, error) {
	return git.GetRepoRoot(p.repoRoot)
}

func (p *GitProvider) CommitAll(ctx context.Context, worktreePath, message string) error {
	return git.CommitAll(worktreePath, message)
}

func (p *GitProvider) MergeBranch(ctx context.Context, branch string) error {
	return git.MergeBranch(p.repoRoot, branch)
}

func (p *GitProvider) DeleteBranch(ctx context.Context, branch string) error {
	return git.DeleteBranch(p.repoRoot, branch)
}
