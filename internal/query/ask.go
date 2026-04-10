package query

import (
	"context"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
)

// AskConfig holds parameters for the Ask compound operation.
type AskConfig struct {
	Query            string
	Budget           int
	MaxItems         int
	SearchLimit      int
	ActivationScores map[string]float64
}

// AskResult is the combined output of a search + context-pack operation.
type AskResult struct {
	Query   string                    `json:"query"`
	TopHits []ScoredResult            `json:"top_hits"`
	Context *memory.ContextPackResult `json:"context,omitempty"`
}

// Ask searches the vault for the query, then packs token-budgeted context
// around the top hit. Context-pack failure is non-fatal: search results are
// returned even if context-pack cannot resolve the top hit.
func Ask(retriever Retriever, resolver *graph.Resolver, db *index.DB, cfg AskConfig) (*AskResult, error) {
	hits, _, err := retriever.Search(context.Background(), cfg.Query, cfg.SearchLimit, 0, index.SearchFilters{})
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

	packResult, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input:            hits[0].ID,
		Budget:           cfg.Budget,
		Depth:            1,
		MaxItems:         cfg.MaxItems,
		Slim:             true,
		ActivationScores: cfg.ActivationScores,
	})
	if err != nil {
		// Non-fatal: return search results without context.
		return result, nil //nolint:nilerr
	}

	result.Context = packResult
	return result, nil
}
