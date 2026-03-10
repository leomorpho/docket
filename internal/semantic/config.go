package semantic

import (
	"path/filepath"

	"github.com/leoaudibert/docket/internal/ticket"
)

func BridgeScriptPath(repoRoot string) string {
	return filepath.Join(repoRoot, "scripts", "semantic_embed.py")
}

func ConfigFromTicket(cfg ticket.SemanticConfig) Config {
	return Config{
		Enabled:                  cfg.Enabled,
		Provider:                 cfg.Provider,
		Model:                    cfg.Model,
		HFHome:                   cfg.HFHome,
		SentenceTransformersHome: cfg.SentenceTransformersHome,
		UVCacheDir:               cfg.UVCacheDir,
		LexicalWeight:            cfg.LexicalWeight,
		VectorWeight:             cfg.VectorWeight,
	}
}
