package query

import (
	"context"

	"github.com/peiman/vaultmind/internal/index"
)

// ScoredResult is a retrieval hit with a normalized score in [0, 1].
type ScoredResult struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Title    string  `json:"title"`
	Path     string  `json:"path"`
	Snippet  string  `json:"snippet"`
	Score    float64 `json:"score"`
	IsDomain bool    `json:"is_domain_note"`
}

// Retriever abstracts a retrieval backend (FTS, embedding, hybrid).
type Retriever interface {
	Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]ScoredResult, int, error)
}
