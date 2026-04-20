package query

import (
	"context"

	"github.com/peiman/vaultmind/internal/index"
)

// ScoredResult is a retrieval hit with a normalized score in [0, 1].
// Components, when populated, carries per-sub-retriever RRF contributions
// from a hybrid retrieval (e.g. {"fts": 0.0164, "dense": 0.0161}). Non-hybrid
// retrievers leave Components nil. The component values sum to Score.
type ScoredResult struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Title      string             `json:"title"`
	Path       string             `json:"path"`
	Snippet    string             `json:"snippet"`
	Score      float64            `json:"score"`
	IsDomain   bool               `json:"is_domain_note"`
	Components map[string]float64 `json:"components,omitempty"`
}

// Retriever abstracts a retrieval backend (FTS, embedding, hybrid).
type Retriever interface {
	Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]ScoredResult, int, error)
}

// NamedRetriever pairs a retrieval backend with a label used for per-component
// score attribution in HybridRetriever. The name appears in ScoredResult.Components.
type NamedRetriever struct {
	Name      string
	Retriever Retriever
}
