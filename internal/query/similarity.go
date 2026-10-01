package query

import (
	"context"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
)

// NoteSimilarities embeds the query and computes cosine similarity against all
// stored note embeddings. Returns noteID -> similarity (0.0-1.0).
// Returns nil map if embedder is nil (keyword-only mode).
func NoteSimilarities(ctx context.Context, queryText string, embedder embedding.Embedder, db *index.DB) (map[string]float64, error) {
	if embedder == nil {
		return nil, nil //nolint:nilnil // nil embedder = keyword-only mode, no similarities to compute
	}
	queryVec, err := embedder.Embed(ctx, queryText)
	if err != nil {
		return nil, err
	}
	all, err := index.LoadAllEmbeddings(db)
	if err != nil {
		return nil, err
	}
	sims := make(map[string]float64, len(all))
	for _, ne := range all {
		sims[ne.NoteID] = CosineSimilarity(queryVec, ne.Embedding)
	}
	return sims, nil
}

// CosineSimilarity is retained as the query-layer name for the shared
// implementation in internal/embedding. One definition, so the two layers that
// need it cannot drift apart (ADR-009 forbids distill importing query).
func CosineSimilarity(a, b []float32) float64 { return embedding.CosineSimilarity(a, b) }
