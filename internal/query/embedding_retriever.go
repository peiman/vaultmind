package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
)

// EmbeddingRetriever searches by cosine similarity between query and stored note embeddings.
type EmbeddingRetriever struct {
	DB       *index.DB
	Embedder embedding.Embedder
}

// snippetMaxLen is the maximum length of a body text snippet in search results.
const snippetMaxLen = 200

// Search embeds the query, computes cosine similarity against all stored embeddings,
// and returns the top results sorted by score descending.
func (r *EmbeddingRetriever) Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	queryVec, err := r.Embedder.Embed(ctx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("embedding query: %w", err)
	}

	// Units, not notes: a long note is scored by each of its sections.
	all, err := index.LoadUnitEmbeddings(r.DB)
	if err != nil {
		return nil, 0, fmt.Errorf("loading embeddings: %w", err)
	}
	if len(all) == 0 {
		return nil, 0, nil
	}

	keep, err := noteFilter(r.DB, filters)
	if err != nil {
		return nil, 0, err
	}

	// Score, filter, and build results in one pass
	var results []retrieval.ScoredResult
	for _, ne := range all {
		if !keep(ne.NoteID, ne.Type) {
			continue
		}
		results = append(results, retrieval.ScoredResult{
			ID:       ne.NoteID,
			Section:  ne.SectionID,
			Type:     ne.Type,
			Title:    ne.Title,
			Path:     ne.Path,
			Snippet:  truncate(ne.BodyText, snippetMaxLen),
			Score:    CosineSimilarity(queryVec, ne.Embedding),
			IsDomain: ne.IsDomain,
		})
	}

	// Sort by score descending, then one hit per note (its best section).
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	out, total := page(firstPerNote(results), limit, offset)
	return out, total, nil
}

// truncate returns the first n bytes of s, breaking at a space boundary if possible.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Find last space within limit for clean break
	cut := s[:n]
	for i := len(cut) - 1; i > n-30; i-- {
		if cut[i] == ' ' {
			return cut[:i] + "..."
		}
	}
	return cut + "..."
}
