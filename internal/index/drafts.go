package index

import "database/sql"

// RowQuery is the read surface DraftIDs needs. *DB satisfies it, and so does
// any other querier with the same Query method.
type RowQuery interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// DraftSet is every note id whose status is exactly StatusDraft.
// Ranking and graph delivery both load this set, so "draft" stays one query
// (Principle 7) instead of a status predicate copied into each path.
type DraftSet map[string]struct{}

// DraftIDs loads the draft set. The set is never nil. A nil db yields an
// empty set: there is nothing to hide.
func DraftIDs(db RowQuery) (DraftSet, error) {
	set := DraftSet{}
	if db == nil {
		return set, nil
	}
	rows, err := db.Query(`SELECT id FROM notes WHERE status = ?`, StatusDraft)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		set[id] = struct{}{}
	}
	return set, rows.Err()
}

// Contains reports whether id is a draft. A nil set contains nothing, so a
// caller that forgot to load the set must not treat that as "hide everyone".
func (s DraftSet) Contains(id string) bool {
	if len(s) == 0 {
		return false
	}
	_, ok := s[id]
	return ok
}

// Visible reports whether id may be delivered as a neighbor, a context item,
// or a recited note. include is the caller's opt-in
// (app.retrieval.include_drafts). An id the caller named is not passed here:
// asking for that note is a direct read.
func (s DraftSet) Visible(id string, include bool) bool {
	return include || !s.Contains(id)
}
