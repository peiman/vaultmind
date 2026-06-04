package query

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
)

// SearchResult is the JSON response for search.
type SearchResult struct {
	Query  string                   `json:"query"`
	Offset int                      `json:"offset"`
	Limit  int                      `json:"limit"`
	Hits   []retrieval.ScoredResult `json:"hits"`
	Total  int                      `json:"total"`
}

// SearchConfig holds search parameters.
type SearchConfig struct {
	Query      string
	Limit      int
	Offset     int
	TypeFilter string
	TagFilter  string
	JSONOutput bool
	VaultPath  string
}

// RunSearch executes the search command logic and returns the result for
// downstream use (e.g. experiment logging). Rendering is still written to w.
func RunSearch(retriever retrieval.Retriever, cfg SearchConfig, w io.Writer) (*SearchResult, error) {
	results, total, err := retriever.Search(
		context.Background(), cfg.Query, cfg.Limit, cfg.Offset,
		index.SearchFilters{Type: cfg.TypeFilter, Tag: cfg.TagFilter},
	)
	if err != nil {
		return nil, fmt.Errorf("searching: %w", err)
	}

	if results == nil {
		results = []retrieval.ScoredResult{}
	}

	out := &SearchResult{
		Query: cfg.Query, Offset: cfg.Offset, Limit: cfg.Limit,
		Hits: results, Total: total,
	}

	if cfg.JSONOutput {
		env := envelope.OK("search", out)
		env.Meta.VaultPath = cfg.VaultPath
		if err := json.NewEncoder(w).Encode(env); err != nil {
			return out, err
		}
		return out, nil
	}

	for _, r := range results {
		if _, err := fmt.Fprintf(w, "%s  %s\n", r.ID, r.Title); err != nil {
			return out, err
		}
	}
	return out, nil
}
