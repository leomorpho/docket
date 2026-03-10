package semantic

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Config struct {
	Enabled                  bool
	Provider                 string
	Model                    string
	HFHome                   string
	SentenceTransformersHome string
	UVCacheDir               string
	LexicalWeight            float64
	VectorWeight             float64
}

type Input struct {
	ChunkID  string `json:"chunk_id"`
	TicketID string `json:"ticket_id,omitempty"`
	Field    string `json:"field"`
	Text     string `json:"text"`
}

type EmbedRequest struct {
	Model  string  `json:"model"`
	Inputs []Input `json:"inputs"`
}

type EmbedResult struct {
	ChunkID string    `json:"chunk_id"`
	Vector  []float64 `json:"vector"`
}

type ResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	ChunkID string `json:"chunk_id,omitempty"`
}

type EmbedResponse struct {
	Model     string          `json:"model"`
	Dimension int             `json:"dimension"`
	Results   []EmbedResult   `json:"results,omitempty"`
	Errors    []ResponseError `json:"errors,omitempty"`
}

type Status struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Available bool   `json:"available"`
	Details   string `json:"details,omitempty"`
}

const (
	EnvSemanticEnabled                  = "DOCKET_SEMANTIC_ENABLED"
	EnvSemanticProvider                 = "DOCKET_SEMANTIC_PROVIDER"
	EnvSemanticModel                    = "DOCKET_SEMANTIC_MODEL"
	EnvSemanticHFHome                   = "DOCKET_SEMANTIC_HF_HOME"
	EnvSemanticSentenceTransformersHome = "DOCKET_SEMANTIC_SENTENCE_TRANSFORMERS_HOME"
	EnvSemanticUVCacheDir               = "DOCKET_SEMANTIC_UV_CACHE_DIR"
	EnvSemanticLexicalWeight            = "DOCKET_SEMANTIC_LEXICAL_WEIGHT"
	EnvSemanticVectorWeight             = "DOCKET_SEMANTIC_VECTOR_WEIGHT"
)

var UVPinnedPackages = []string{
	"sentence-transformers==3.4.1",
}

type Provider interface {
	Name() string
	Status(context.Context) (Status, error)
	Embed(context.Context, EmbedRequest) (EmbedResponse, error)
}

type ProviderOptions struct {
	RepoRoot string
	Runner   Runner
}

var ErrUnknownProvider = errors.New("unknown semantic provider")

type ProviderErrorKind string

const (
	ProviderErrorTimeout     ProviderErrorKind = "timeout"
	ProviderErrorInvalidJSON ProviderErrorKind = "invalid_json"
	ProviderErrorBridge      ProviderErrorKind = "bridge_error"
	ProviderErrorProcess     ProviderErrorKind = "process_error"
)

type ProviderError struct {
	Kind     ProviderErrorKind
	Message  string
	Stderr   string
	Response *EmbedResponse
	Err      error
}

func (e *ProviderError) Error() string {
	var parts []string
	parts = append(parts, string(e.Kind))
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	if e.Stderr != "" {
		parts = append(parts, e.Stderr)
	}
	return strings.Join(parts, ": ")
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

func NewProvider(cfg Config, opts ProviderOptions) (Provider, error) {
	switch cfg.Provider {
	case "", "uv":
		return NewUVProvider(cfg, opts), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, cfg.Provider)
	}
}
