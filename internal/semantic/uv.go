package semantic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

type UVProvider struct {
	cfg        Config
	repoRoot   string
	scriptPath string
	runner     Runner
}

func NewUVProvider(cfg Config, opts ProviderOptions) *UVProvider {
	runner := opts.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	return &UVProvider{
		cfg:        cfg,
		repoRoot:   opts.RepoRoot,
		scriptPath: BridgeScriptPath(opts.RepoRoot),
		runner:     runner,
	}
}

func (p *UVProvider) Name() string {
	return "uv"
}

func (p *UVProvider) Status(context.Context) (Status, error) {
	return Status{
		Provider:  p.Name(),
		Model:     p.cfg.Model,
		Available: false,
		Details:   "uv availability not checked",
	}, nil
}

func (p *UVProvider) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	result, err := p.runBridge(ctx, req)
	if err != nil {
		return EmbedResponse{}, err
	}

	var response EmbedResponse
	if err := json.Unmarshal(result.Stdout, &response); err != nil {
		return EmbedResponse{}, fmt.Errorf("decode bridge response: %w", err)
	}
	return response, nil
}

func (p *UVProvider) runBridge(ctx context.Context, req EmbedRequest) (CommandResult, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return CommandResult{}, fmt.Errorf("encode bridge request: %w", err)
	}
	return p.runner.Run(ctx, CommandSpec{
		Path:  "uv",
		Args:  []string{"run", "--no-project", "python", p.scriptPath, "--model", p.cfg.Model},
		Env:   p.commandEnv(),
		Dir:   p.repoRoot,
		Stdin: append(payload, '\n'),
	})
}

func (p *UVProvider) commandEnv() []string {
	env := os.Environ()
	env = append(env, "HF_HOME="+p.cfg.HFHome)
	env = append(env, "SENTENCE_TRANSFORMERS_HOME="+p.cfg.SentenceTransformersHome)
	env = append(env, "UV_CACHE_DIR="+p.cfg.UVCacheDir)
	return env
}
