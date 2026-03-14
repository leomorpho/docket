package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/vcs"
	"github.com/leomorpho/docket/internal/workflow"
)

type Request struct {
	ID     interface{}            `json:"id,omitempty"`
	Action string                 `json:"action"`
	Args   map[string]interface{} `json:"args,omitempty"`
}

type Response struct {
	ID     interface{} `json:"id,omitempty"`
	OK     bool        `json:"ok"`
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

func ServeMCP(in io.Reader, out io.Writer, repoRoot string) error {
	deps, err := buildDispatchDeps(repoRoot)
	if err != nil {
		return err
	}
	return ServeMCPWithDeps(in, out, deps)
}

func ServeMCPWithDeps(in io.Reader, out io.Writer, deps *DispatchDeps) error {
	s := bufio.NewScanner(in)
	w := bufio.NewWriter(out)
	defer w.Flush()

	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp := Response{OK: false, Error: fmt.Sprintf("invalid json: %v", err)}
			if err := writeResponse(w, resp); err != nil {
				return err
			}
			continue
		}

		result, err := Dispatch(req.Action, req.Args, deps)
		resp := Response{ID: req.ID}
		if err != nil {
			resp.OK = false
			resp.Error = err.Error()
		} else {
			resp.OK = true
			resp.Result = result
		}
		if err := writeResponse(w, resp); err != nil {
			return err
		}
	}

	if err := s.Err(); err != nil {
		return err
	}
	return nil
}

type claimLookupAdapter struct {
	manager claim.Manager
}

func (a *claimLookupAdapter) GetClaim(ctx context.Context, ticketID string) (*ClaimMetadata, error) {
	cl, err := a.manager.GetClaim(ctx, ticketID)
	if err != nil || cl == nil {
		return nil, err
	}
	return &ClaimMetadata{
		AgentID:  cl.AgentID,
		Worktree: cl.Worktree,
	}, nil
}

func buildDispatchDeps(repoRoot string) (*DispatchDeps, error) {
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	s := local.New(repoRoot)
	claimMgr := claim.NewLocalClaimManager(repoRoot)
	return NewDispatchDeps(repoRoot, s, workflow.NewManager(s, vcs.NewGitProvider(repoRoot), claimMgr), claimMgr, cfg), nil
}

func NewDispatchDeps(repoRoot string, s store.Backend, wf WorkflowRunner, claimMgr claim.Manager, cfg *ticket.Config) *DispatchDeps {
	return &DispatchDeps{
		RepoRoot: repoRoot,
		Store:    s,
		Workflow: wf,
		Claimer:  &claimLookupAdapter{manager: claimMgr},
		Config:   cfg,
	}
}

func writeResponse(w *bufio.Writer, resp Response) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return err
	}
	return w.Flush()
}
