package query

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
)

// SearchResult is the JSON response for search.
type SearchResult struct {
	Query  string         `json:"query"`
	Offset int            `json:"offset"`
	Limit  int            `json:"limit"`
	Hits   []ScoredResult `json:"hits"`
	Total  int            `json:"total"`
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

// RunSearch executes the search command logic.
func RunSearch(retriever Retriever, cfg SearchConfig, w io.Writer) error {
	results, total, err := retriever.Search(
		context.Background(), cfg.Query, cfg.Limit, cfg.Offset,
		index.SearchFilters{Type: cfg.TypeFilter, Tag: cfg.TagFilter},
	)
	if err != nil {
		return fmt.Errorf("searching: %w", err)
	}

	if results == nil {
		results = []ScoredResult{}
	}

	if cfg.JSONOutput {
		env := envelope.OK("search", SearchResult{
			Query: cfg.Query, Offset: cfg.Offset, Limit: cfg.Limit,
			Hits: results, Total: total,
		})
		env.Meta.VaultPath = cfg.VaultPath
		return json.NewEncoder(w).Encode(env)
	}

	for _, r := range results {
		if _, err := fmt.Fprintf(w, "%s  %s\n", r.ID, r.Title); err != nil {
			return err
		}
	}
	return nil
}
