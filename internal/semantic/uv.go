package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	uvStatusTimeout = 2 * time.Second
	uvEmbedTimeout  = 2 * time.Minute
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

func (p *UVProvider) Status(ctx context.Context) (Status, error) {
	ctx, cancel := context.WithTimeout(ctx, uvStatusTimeout)
	defer cancel()

	result, err := p.runner.Run(ctx, CommandSpec{
		Path: "uv",
		Args: []string{"--version"},
		Env:  p.commandEnv(),
		Dir:  p.repoRoot,
	})
	if err != nil {
		return Status{
			Provider:  p.Name(),
			Model:     p.cfg.Model,
			Available: false,
			Details:   errorDetails(err, result.Stderr),
		}, nil
	}

	return Status{
		Provider:  p.Name(),
		Model:     p.cfg.Model,
		Available: true,
		Details:   strings.TrimSpace(string(result.Stdout)),
	}, nil
}

func (p *UVProvider) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, uvEmbedTimeout)
	defer cancel()

	result, err := p.runBridge(ctx, req)
	if err != nil {
		return EmbedResponse{}, p.classifyBridgeError(result, err)
	}

	response, err := decodeBridgeResponse(result.Stdout)
	if err != nil {
		return EmbedResponse{}, &ProviderError{
			Kind:    ProviderErrorInvalidJSON,
			Message: "decode bridge response",
			Stderr:  strings.TrimSpace(string(result.Stderr)),
			Err:     err,
		}
	}
	if len(response.Errors) > 0 {
		return EmbedResponse{}, &ProviderError{
			Kind:     ProviderErrorBridge,
			Message:  firstBridgeMessage(response),
			Stderr:   strings.TrimSpace(string(result.Stderr)),
			Response: &response,
		}
	}
	return response, nil
}

func (p *UVProvider) runBridge(ctx context.Context, req EmbedRequest) (CommandResult, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return CommandResult{}, fmt.Errorf("encode bridge request: %w", err)
	}
	args := []string{"run", "--no-project"}
	for _, pkg := range UVPinnedPackages {
		args = append(args, "--with", pkg)
	}
	args = append(args, "python", p.scriptPath, "--model", p.cfg.Model)

	return p.runner.Run(ctx, CommandSpec{
		Path:  "uv",
		Args:  args,
		Env:   p.commandEnv(),
		Dir:   p.repoRoot,
		Stdin: append(payload, '\n'),
	})
}

func (p *UVProvider) commandEnv() []string {
	env := os.Environ()
	env = setEnv(env, "HF_HOME", p.cfg.HFHome)
	env = setEnv(env, "SENTENCE_TRANSFORMERS_HOME", p.cfg.SentenceTransformersHome)
	env = setEnv(env, "UV_CACHE_DIR", p.cfg.UVCacheDir)
	return env
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	replaced := false
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			replaced = true
		}
	}
	if !replaced {
		env = append(env, prefix+value)
	}
	return env
}

func errorDetails(err error, stderr []byte) string {
	if len(stderr) > 0 {
		return strings.TrimSpace(string(stderr))
	}
	return err.Error()
}

func decodeBridgeResponse(stdout []byte) (EmbedResponse, error) {
	var response EmbedResponse
	if err := json.Unmarshal(stdout, &response); err != nil {
		return EmbedResponse{}, err
	}
	return response, nil
}

func (p *UVProvider) classifyBridgeError(result CommandResult, err error) error {
	stderr := strings.TrimSpace(string(result.Stderr))
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &ProviderError{
			Kind:    ProviderErrorTimeout,
			Message: "semantic provider timed out",
			Stderr:  stderr,
			Err:     err,
		}
	}

	if len(result.Stdout) > 0 {
		response, decodeErr := decodeBridgeResponse(result.Stdout)
		if decodeErr == nil {
			return &ProviderError{
				Kind:     ProviderErrorBridge,
				Message:  firstBridgeMessage(response),
				Stderr:   stderr,
				Response: &response,
				Err:      err,
			}
		}
		return &ProviderError{
			Kind:    ProviderErrorInvalidJSON,
			Message: "decode bridge response",
			Stderr:  stderr,
			Err:     decodeErr,
		}
	}

	return &ProviderError{
		Kind:    ProviderErrorProcess,
		Message: "semantic provider process failed",
		Stderr:  stderr,
		Err:     err,
	}
}

func firstBridgeMessage(response EmbedResponse) string {
	if len(response.Errors) == 0 {
		return "bridge returned error response"
	}
	return response.Errors[0].Message
}
