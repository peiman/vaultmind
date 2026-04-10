package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
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
func (r *EmbeddingRetriever) Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]ScoredResult, int, error) {
	queryVec, err := r.Embedder.Embed(ctx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("embedding query: %w", err)
	}

	all, err := index.LoadAllEmbeddings(r.DB)
	if err != nil {
		return nil, 0, fmt.Errorf("loading embeddings: %w", err)
	}
	if len(all) == 0 {
		return nil, 0, nil
	}

	// Build tag lookup if tag filter is active (one query instead of N)
	var noteTags map[string]bool
	if filters.Tag != "" {
		noteTags, err = r.noteIDsWithTag(filters.Tag)
		if err != nil {
			return nil, 0, fmt.Errorf("loading tags: %w", err)
		}
	}

	// Score, filter, and build results in one pass
	type scored struct {
		result ScoredResult
		score  float64
	}
	var results []scored
	for _, ne := range all {
		if filters.Type != "" && ne.Type != filters.Type {
			continue
		}
		if filters.Tag != "" && !noteTags[ne.NoteID] {
			continue
		}
		sim := CosineSimilarity(queryVec, ne.Embedding)
		results = append(results, scored{
			result: ScoredResult{
				ID:       ne.NoteID,
				Type:     ne.Type,
				Title:    ne.Title,
				Path:     ne.Path,
				Snippet:  truncate(ne.BodyText, snippetMaxLen),
				Score:    sim,
				IsDomain: ne.IsDomain,
			},
			score: sim,
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	total := len(results)

	// Apply offset/limit
	if offset >= len(results) {
		return nil, total, nil
	}
	results = results[offset:]
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	out := make([]ScoredResult, len(results))
	for i, s := range results {
		out[i] = s.result
	}
	return out, total, nil
}

// noteIDsWithTag returns the set of note IDs that have the given tag.
func (r *EmbeddingRetriever) noteIDsWithTag(tag string) (map[string]bool, error) {
	rows, err := r.DB.Query("SELECT note_id FROM tags WHERE tag = ?", tag)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
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
