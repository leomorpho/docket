package semantic

import (
	"context"
	"errors"
	"fmt"
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

func NewProvider(cfg Config, opts ProviderOptions) (Provider, error) {
	switch cfg.Provider {
	case "", "uv":
		return NewUVProvider(cfg, opts), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, cfg.Provider)
	}
}
