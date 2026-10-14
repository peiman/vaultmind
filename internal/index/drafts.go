package index

import "database/sql"

// RowQuery is the read surface DraftIDs needs. *DB satisfies it, and so does
// any other querier with the same Query method.
type RowQuery interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// TypeGap is the note type of a question the vault did not answer.
// A gap is not knowledge. vault.GapType is the same string: the two
// packages cannot import each other, and a vault test locks them.
const TypeGap = "gap"

// NonKnowledge is every note that must not be delivered as knowledge.
// Drafts are hidden until the caller opts in. Gaps are always hidden.
// One set, so ranking and graph delivery cannot disagree (Principle 7).
//
// DraftSet is the historical name. Call sites and the ADR-102 tests keep it.
type NonKnowledge struct {
	drafts map[string]struct{}
	gaps   map[string]struct{}
}

// DraftSet is the non-knowledge set.
type DraftSet = NonKnowledge

// DraftIDs loads the non-knowledge set: status draft, or type gap.
// The set is never nil. A nil db yields an empty set: there is nothing to hide.
func DraftIDs(db RowQuery) (DraftSet, error) {
	set := DraftSet{
		drafts: map[string]struct{}{},
		gaps:   map[string]struct{}{},
	}
	if db == nil {
		return set, nil
	}
	rows, err := db.Query(
		`SELECT id, IFNULL(status, ''), IFNULL(type, '') FROM notes WHERE status = ? OR type = ?`,
		StatusDraft, TypeGap)
	if err != nil {
		return DraftSet{}, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, status, noteType string
		if err := rows.Scan(&id, &status, &noteType); err != nil {
			return DraftSet{}, err
		}
		if status == StatusDraft {
			set.drafts[id] = struct{}{}
		}
		if noteType == TypeGap {
			set.gaps[id] = struct{}{}
		}
	}
	return set, rows.Err()
}

// Contains reports whether id is a draft or a gap. A nil set contains nothing,
// so a caller that forgot to load the set must not treat that as "hide everyone".
func (s DraftSet) Contains(id string) bool {
	if len(s.drafts) == 0 && len(s.gaps) == 0 {
		return false
	}
	if _, ok := s.drafts[id]; ok {
		return true
	}
	_, ok := s.gaps[id]
	return ok
}

// Visible reports whether id may be delivered as a neighbor, a context item,
// or a recited note. include is the caller's draft opt-in
// (app.retrieval.include_drafts). It never reveals a gap. An id the caller
// named is not passed here: asking for that note is a direct read.
func (s DraftSet) Visible(id string, include bool) bool {
	if _, ok := s.gaps[id]; ok {
		return false
	}
	if _, ok := s.drafts[id]; ok {
		return include
	}
	return true
}
