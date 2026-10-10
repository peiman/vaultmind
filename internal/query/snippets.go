package query

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
)

// populateRawSnippets reads delivery text only after scoring and pagination.
// body_raw is appended after potentially large ColBERT blobs in SQLite rows;
// loading it for every scoring candidate would traverse those overflow pages.
func populateRawSnippets(db *index.DB, results []retrieval.ScoredResult) error {
	for i := range results {
		var body string
		err := db.QueryRow("SELECT COALESCE(body_raw, '') FROM notes WHERE id = ?", results[i].ID).Scan(&body)
		if errors.Is(err, sql.ErrNoRows) {
			continue // A concurrent index pass removed the note after scoring.
		}
		if err != nil {
			return fmt.Errorf("reading raw body preview for %q: %w", results[i].ID, err)
		}
		results[i].Snippet = truncate(body, snippetMaxLen)
	}
	return nil
}
