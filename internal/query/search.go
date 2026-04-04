package query

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
)

// SearchResult is the JSON response for search.
type SearchResult struct {
	Query  string            `json:"query"`
	Offset int               `json:"offset"`
	Limit  int               `json:"limit"`
	Hits   []index.FTSResult `json:"hits"`
	Total  int               `json:"total"`
}

// RunSearch executes the search command logic.
func RunSearch(db *index.DB, queryStr, vaultPath string, limit, offset int, jsonOut bool, w io.Writer) error {
	results, err := index.SearchFTS(db, queryStr, limit, offset)
	if err != nil {
		return fmt.Errorf("searching: %w", err)
	}

	if jsonOut {
		env := envelope.OK("search", SearchResult{
			Query: queryStr, Offset: offset, Limit: limit,
			Hits: results, Total: len(results),
		})
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(w).Encode(env)
	}

	for _, r := range results {
		if _, err := fmt.Fprintf(w, "%s  %s\n", r.ID, r.Title); err != nil {
			return err
		}
	}
	return nil
}
