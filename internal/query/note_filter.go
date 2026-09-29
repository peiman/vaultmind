package query

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/index"
)

// noteFilter returns the predicate every retrieval lane applies for the type,
// tag and path filters. One definition, because a lane that judged a note
// differently fed it into the fusion anyway: three lanes once ignored the tag,
// so a tag-filtered hybrid search returned notes without it. The tag set and
// the path set are loaded once per search, and only when asked for.
func noteFilter(db *index.DB, filters index.SearchFilters) (func(id, noteType string) bool, error) {
	var tagged map[string]bool
	if filters.Tag != "" {
		var err error
		if tagged, err = noteIDsWithTag(db, filters.Tag); err != nil {
			return nil, fmt.Errorf("loading tags: %w", err)
		}
	}
	var underPrefix map[string]bool
	if filters.PathPrefix != "" {
		var err error
		if underPrefix, err = noteIDsUnderPrefix(db, filters.PathPrefix); err != nil {
			return nil, fmt.Errorf("loading path prefix: %w", err)
		}
	}
	return func(id, noteType string) bool {
		if filters.Type != "" && noteType != filters.Type {
			return false
		}
		if filters.PathPrefix != "" && !underPrefix[id] {
			return false
		}
		return filters.Tag == "" || tagged[id]
	}, nil
}

// noteIDsUnderPrefix returns the set of note IDs whose vault path starts with
// prefix. %, _ and \ in the prefix match literally.
func noteIDsUnderPrefix(db *index.DB, prefix string) (map[string]bool, error) {
	rows, err := db.Query(`SELECT id FROM notes WHERE path LIKE ? ESCAPE '\'`, index.PathPrefixLike(prefix))
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
