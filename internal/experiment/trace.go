package experiment

import (
	"encoding/json"
	"fmt"
	"sort"
)

// RetrievalEventSummary describes one retrieval event — its type, timestamp,
// query, and the deduplicated list of notes returned. Multiple variants of
// the same event collapse: a note appearing under "hybrid" and three shadow
// variants is reported once with its best (lowest) rank across them.
type RetrievalEventSummary struct {
	EventID   string
	EventType string
	Timestamp string
	Query     string
	Hits      []RetrievalEventHit
}

// RetrievalEventHit is one note surfaced by one event, with its rank in the
// event's result set. Rank is the minimum across all variants that contained
// the note (best placement wins).
type RetrievalEventHit struct {
	NoteID string
	Rank   int
}

// SessionHit is one occurrence of a note being retrieved: which session saw
// it, when, at what rank, under which event. Used by NoteRetrievals to build
// a cross-session history for a single note.
type SessionHit struct {
	SessionID string
	EventID   string
	EventType string
	Timestamp string
	Rank      int
}

// SessionRetrievals returns every retrieval event in the given session,
// chronologically ascending. Events of type search / ask / context_pack are
// included; note_access and index_embed are not (they are not retrievals).
// An unknown session ID returns an empty slice without error.
func (d *DB) SessionRetrievals(sessionID string) ([]RetrievalEventSummary, error) {
	rows, err := d.db.Query(
		`SELECT event_id, event_type, timestamp, COALESCE(query_text, ''), event_data
		 FROM events
		 WHERE session_id = ? AND event_type IN ('search', 'ask', 'context_pack')
		 ORDER BY timestamp ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying session retrievals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []RetrievalEventSummary
	for rows.Next() {
		var s RetrievalEventSummary
		var dataStr string
		if err := rows.Scan(&s.EventID, &s.EventType, &s.Timestamp, &s.Query, &dataStr); err != nil {
			return nil, fmt.Errorf("scanning session retrieval row: %w", err)
		}
		s.Hits = parseEventHits(dataStr)
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating session retrieval rows: %w", err)
	}
	return out, nil
}

// NoteRetrievals returns every (session, event, rank, timestamp) tuple where
// the given note appeared in a retrieval. Ordered chronologically. Best rank
// across variants wins when a note appears under multiple variants of one
// event — so one event contributes one row per distinct note, not per variant.
func (d *DB) NoteRetrievals(noteID string) ([]SessionHit, error) {
	rows, err := d.db.Query(
		`SELECT session_id, event_id, event_type, timestamp, event_data
		 FROM events
		 WHERE event_type IN ('search', 'ask', 'context_pack')
		   AND json_valid(event_data)
		 ORDER BY timestamp ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying note retrievals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SessionHit
	for rows.Next() {
		var sessionID, eventID, eventType, timestamp, dataStr string
		if err := rows.Scan(&sessionID, &eventID, &eventType, &timestamp, &dataStr); err != nil {
			return nil, fmt.Errorf("scanning note retrieval row: %w", err)
		}
		hits := parseEventHits(dataStr)
		for _, h := range hits {
			if h.NoteID == noteID {
				out = append(out, SessionHit{
					SessionID: sessionID,
					EventID:   eventID,
					EventType: eventType,
					Timestamp: timestamp,
					Rank:      h.Rank,
				})
				break // one row per event even if the note appears (it shouldn't after dedup)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating note retrieval rows: %w", err)
	}
	return out, nil
}

// parseEventHits extracts (note_id, best rank) pairs from event_data's
// variants map. Shadow variants containing the same note collapse to a
// single hit with the minimum rank. Malformed JSON returns an empty slice
// (callers can still aggregate across other well-formed events).
func parseEventHits(eventData string) []RetrievalEventHit {
	var parsed struct {
		Variants map[string]struct {
			Results []struct {
				NoteID string `json:"note_id"`
				Rank   int    `json:"rank"`
			} `json:"results"`
		} `json:"variants"`
	}
	if err := json.Unmarshal([]byte(eventData), &parsed); err != nil {
		return nil
	}
	bestRank := make(map[string]int)
	for _, variant := range parsed.Variants {
		for _, r := range variant.Results {
			if r.NoteID == "" {
				continue
			}
			if existing, ok := bestRank[r.NoteID]; !ok || r.Rank < existing {
				bestRank[r.NoteID] = r.Rank
			}
		}
	}
	hits := make([]RetrievalEventHit, 0, len(bestRank))
	for id, rank := range bestRank {
		hits = append(hits, RetrievalEventHit{NoteID: id, Rank: rank})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Rank < hits[j].Rank })
	return hits
}
