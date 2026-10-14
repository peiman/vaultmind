package query

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/index"
)

// noteFilter returns the predicate every in-memory retrieval lane applies for
// the type, tag, path and draft filters. FTS applies the same policy in SQL
// (appendSearchFilters) because filtering after LIMIT would drop the top hit
// and return a short page. Both sites ask filters.ExcludesDrafts, so a lane
// cannot rank a draft the others hid. The sets are loaded once per search,
// and only when asked for.
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
	var drafts index.DraftSet
	if filters.ExcludesDrafts() {
		var err error
		if drafts, err = index.DraftIDs(db); err != nil {
			return nil, fmt.Errorf("loading drafts: %w", err)
		}
	}
	return func(id, noteType string) bool {
		if drafts.Contains(id) {
			return false
		}
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
