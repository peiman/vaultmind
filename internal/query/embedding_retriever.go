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

type scoredNote struct {
	noteID string
	score  float64
}

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

	// Score all notes
	scored := make([]scoredNote, len(all))
	for i, ne := range all {
		scored[i] = scoredNote{
			noteID: ne.NoteID,
			score:  CosineSimilarity(queryVec, ne.Embedding),
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Look up metadata and apply filters
	var filtered []ScoredResult
	for _, sn := range scored {
		row, err := r.DB.QueryNoteByID(sn.noteID)
		if err != nil || row == nil {
			continue
		}
		if filters.Type != "" && row.Type != filters.Type {
			continue
		}
		if filters.Tag != "" {
			hasTag, tagErr := r.noteHasTag(sn.noteID, filters.Tag)
			if tagErr != nil || !hasTag {
				continue
			}
		}
		filtered = append(filtered, ScoredResult{
			ID:       row.ID,
			Type:     row.Type,
			Title:    row.Title,
			Path:     row.Path,
			Score:    sn.score,
			IsDomain: row.IsDomain,
		})
	}

	total := len(filtered)

	// Apply offset/limit
	if offset >= len(filtered) {
		return nil, total, nil
	}
	filtered = filtered[offset:]
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered, total, nil
}

func (r *EmbeddingRetriever) noteHasTag(noteID, tag string) (bool, error) {
	var count int
	err := r.DB.QueryRow("SELECT COUNT(*) FROM tags WHERE note_id = ? AND tag = ?", noteID, tag).Scan(&count)
	return count > 0, err
}
