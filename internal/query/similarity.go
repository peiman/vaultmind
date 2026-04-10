package query

import (
	"context"
	"math"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
)

// NoteSimilarities embeds the query and computes cosine similarity against all
// stored note embeddings. Returns noteID -> similarity (0.0-1.0).
// Returns nil map if embedder is nil (keyword-only mode).
func NoteSimilarities(ctx context.Context, queryText string, embedder embedding.Embedder, db *index.DB) (map[string]float64, error) {
	if embedder == nil {
		return nil, nil
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

// CosineSimilarity computes the cosine similarity between two float32 vectors.
// Returns 0 if vectors have different lengths, zero length, or zero magnitude.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
