package cmd

import (
	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/vcs"
	"github.com/leomorpho/docket/internal/workflow"
)

type runtimeDeps struct {
	store    *local.Store
	vcs      *vcs.GitProvider
	claimer  *claim.LocalClaimManager
	workflow *workflow.WorkflowManager
}

func newRuntimeDeps(repoRoot string) *runtimeDeps {
	s := local.New(repoRoot)
	vcsProvider := vcs.NewGitProvider(repoRoot)
	claimMgr := claim.NewLocalClaimManager(repoRoot)
	return &runtimeDeps{
		store:    s,
		vcs:      vcsProvider,
		claimer:  claimMgr,
		workflow: workflow.NewManager(s, vcsProvider, claimMgr),
	}
}
