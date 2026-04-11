package query

import (
	"context"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/rs/zerolog/log"
)

// AskConfig holds parameters for the Ask compound operation.
type AskConfig struct {
	Query            string
	Budget           int
	MaxItems         int
	SearchLimit      int
	ActivationScores map[string]float64
	// Embedder is optional. When non-nil, raw cosine similarities are computed
	// and used for spreading activation scoring via ActivationFunc.
	Embedder embedding.Embedder
	// ActivationFunc optionally recomputes activation scores after similarities
	// are known. When provided and similarities are available, the returned
	// scores replace ActivationScores for context-pack sorting. This enables
	// spreading activation without coupling query to the experiment package.
	ActivationFunc func(similarities map[string]float64) map[string]float64
}

// AskResult is the combined output of a search + context-pack operation.
type AskResult struct {
	Query        string                    `json:"query"`
	TopHits      []ScoredResult            `json:"top_hits"`
	Context      *memory.ContextPackResult `json:"context,omitempty"`
	Similarities map[string]float64        `json:"-"` // raw cosine similarities (not serialized)
}

// Ask searches the vault for the query, computes raw cosine similarities
// (when an embedder is available), recomputes activation scores with
// spreading activation (via ActivationFunc), then packs token-budgeted
// context around the top hit. Context-pack failure is non-fatal.
func Ask(ctx context.Context, retriever Retriever, resolver *graph.Resolver, db *index.DB, cfg AskConfig) (*AskResult, error) {
	hits, _, err := retriever.Search(ctx, cfg.Query, cfg.SearchLimit, 0, index.SearchFilters{})
	if err != nil {
		return nil, err
	}

	result := &AskResult{
		Query:   cfg.Query,
		TopHits: hits,
	}

	if len(hits) == 0 {
		return result, nil
	}

	// Compute raw cosine similarities for spreading activation.
	if cfg.Embedder != nil {
		sims, simErr := NoteSimilarities(ctx, cfg.Query, cfg.Embedder, db)
		if simErr != nil {
			log.Debug().Err(simErr).Msg("note similarities unavailable; spreading activation disabled for this query")
		} else {
			result.Similarities = sims
		}
	}

	// Recompute activation scores with similarity data when available.
	activationScores := cfg.ActivationScores
	if result.Similarities != nil && cfg.ActivationFunc != nil {
		if updated := cfg.ActivationFunc(result.Similarities); updated != nil {
			activationScores = updated
		}
	}

	packResult, packErr := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input:            hits[0].ID,
		Budget:           cfg.Budget,
		Depth:            1,
		MaxItems:         cfg.MaxItems,
		Slim:             true,
		ActivationScores: activationScores,
	})
	if packErr != nil {
		log.Debug().Err(packErr).Str("note_id", hits[0].ID).Msg("context-pack failed; returning search results only")
		return result, nil
	}

	result.Context = packResult
	return result, nil
}
