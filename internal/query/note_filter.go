package query

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/index"
)

// noteFilter returns the predicate every retrieval lane applies for the type
// and tag filters. One definition, because a lane that judged a note
// differently fed it into the fusion anyway: three lanes once ignored the tag,
// so a tag-filtered hybrid search returned notes without it. The tag set is
// loaded once per search, and only when a tag is asked for.
func noteFilter(db *index.DB, filters index.SearchFilters) (func(id, noteType string) bool, error) {
	var tagged map[string]bool
	if filters.Tag != "" {
		var err error
		if tagged, err = noteIDsWithTag(db, filters.Tag); err != nil {
			return nil, fmt.Errorf("loading tags: %w", err)
		}
	}
	return func(id, noteType string) bool {
		if filters.Type != "" && noteType != filters.Type {
			return false
		}
		return filters.Tag == "" || tagged[id]
	}, nil
}

// noteIDsWithTag returns the set of note IDs that have the given tag.
func noteIDsWithTag(db *index.DB, tag string) (map[string]bool, error) {
	rows, err := db.Query("SELECT note_id FROM tags WHERE tag = ?", tag)
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
