package experiment

import "fmt"

// NoteStat reports aggregate retrieval metrics for a single note, derived
// from the per_note_stats SQL view over event_data.variants.*.results[].
// Counts are per distinct retrieval event, not per variant occurrence —
// so a note appearing under 3 shadow variants of one event counts once.
type NoteStat struct {
	NoteID              string
	RetrievalCountTotal int
	FirstRetrievedTs    string
	LastRetrievedTs     string
}

// PerNoteStats returns one row per note that has appeared in a retrieval
// event, ordered by retrieval_count_total desc (most-often-retrieved first),
// then by last_retrieved_ts desc to break count ties with recency.
//
// Note: the source-of-truth for which notes exist is the vault index DB, not
// the experiment DB. This query only knows about notes that have been
// retrieved at least once; notes that were never retrieved do not appear.
func (d *DB) PerNoteStats() ([]NoteStat, error) {
	rows, err := d.db.Query(`
		SELECT note_id, retrieval_count_total, first_retrieved_ts, last_retrieved_ts
		FROM per_note_stats
		ORDER BY retrieval_count_total DESC, last_retrieved_ts DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying per_note_stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []NoteStat
	for rows.Next() {
		var s NoteStat
		if err := rows.Scan(&s.NoteID, &s.RetrievalCountTotal, &s.FirstRetrievedTs, &s.LastRetrievedTs); err != nil {
			return nil, fmt.Errorf("scanning per_note_stats row: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating per_note_stats rows: %w", err)
	}
	return out, nil
}
