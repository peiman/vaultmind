package experiment

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// actualResultsSQL selects the actual retrieval lane, never shadow variants.
// Keyword-only vaults use the same measurement as embedding-backed vaults.
const actualResultsSQL = `COALESCE(json_extract(e.event_data, '$.variants.hybrid.results'), json_extract(e.event_data, '$.variants.keyword.results'))`

type surfacedEvidence struct {
	event, session, vault, note, caller string
	at                                  time.Time
}
type openedEvidence struct {
	vault, note string
	at          time.Time
}

// RecordUseEvidence runs during episode capture, off the recall/prompt path.
// It records observations, never an inference that a note was used or unused.
// Re-capture upserts one event per (surfaced event, note), allowing a late read
// or a transcript tail to upgrade evidence without inflating doctor's counts.
func (d *DB) RecordUseEvidence(transcriptPath string) error {
	transcript, err := readEvidenceTranscript(transcriptPath)
	if err != nil {
		return err
	}
	surfaced, err := d.surfacedEvidence(transcript.session)
	if err != nil {
		return err
	}
	reads, err := d.openedEvidence(transcript.session)
	if err != nil {
		return err
	}
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin use evidence: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, s := range surfaced {
		turn := evidenceTurnFor(transcript.turns, s.at)
		if s.caller == "vaultmind-userprompt-hook" && s.at.Nanosecond() == 0 {
			turn = promptEvidenceTurn(transcript.turns, s.at, turn)
		}
		state := "unknown"
		turnID := ""
		until := s.at.Add(consumptionWindow)
		if turn != nil {
			turnID = turn.id
			if !turn.next.IsZero() {
				until = turn.next
			} else if turn.last.After(until) {
				until = turn.last.Add(time.Nanosecond)
			}
			for _, text := range turn.texts {
				if exactEvidenceID(text, s.note) {
					state = "cited"
					break
				}
			}
		}
		for _, r := range reads {
			if r.vault == s.vault && r.note == s.note && !r.at.Before(s.at) && r.at.Before(until) {
				state = "opened"
				break
			}
		}
		data, err := json.Marshal(map[string]string{"note_id": s.note, "evidence": state, "surfaced_event_id": s.event, "turn_id": turnID, "user_session_id": transcript.session})
		if err != nil {
			return fmt.Errorf("marshal use evidence: %w", err)
		}
		id := fmt.Sprintf("use-%x", sha256.Sum256([]byte(s.event+"\x00"+s.note)))
		_, err = tx.Exec(`INSERT INTO events (event_id,session_id,event_type,timestamp,vault_path,event_data)
   VALUES (?,?,?,?,?,?) ON CONFLICT(event_id) DO UPDATE SET event_data=excluded.event_data
   WHERE CASE json_extract(excluded.event_data,'$.evidence') WHEN 'opened' THEN 2 WHEN 'cited' THEN 1 ELSE 0 END
    >= CASE json_extract(events.event_data,'$.evidence') WHEN 'opened' THEN 2 WHEN 'cited' THEN 1 ELSE 0 END`,
			id, s.session, EventUseEvidence, time.Now().UTC().Format(time.RFC3339), s.vault, string(data))
		if err != nil {
			return fmt.Errorf("record use evidence: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit use evidence: %w", err)
	}
	return nil
}

// LogEvent timestamps have whole-second precision. A prompt appended in that
// same second belongs to this recall even if the preceding reply also finished
// in that second. Multiple prompts in one second are ambiguous: stay unknown.
func promptEvidenceTurn(turns []evidenceTurn, at time.Time, fallback *evidenceTurn) *evidenceTurn {
	var match *evidenceTurn
	for i := range turns {
		if turns[i].start.Truncate(time.Second).Equal(at) {
			if match != nil {
				return nil
			}
			match = &turns[i]
		}
	}
	if match != nil {
		return match
	}
	return fallback
}

// A prompt hook can finish before its user record is appended. The idle gap
// after the preceding assistant reply belongs to the upcoming prompt, rather
// than lending the preceding reply's citations to the new recall.
func evidenceTurnFor(turns []evidenceTurn, at time.Time) *evidenceTurn {
	for i := range turns {
		t := &turns[i]
		if !at.Before(t.start) && (t.next.IsZero() || at.Before(t.next)) {
			if !t.last.IsZero() && at.After(t.last) && i+1 < len(turns) {
				return &turns[i+1]
			}
			return t
		}
		if at.Before(t.start) && (i == 0 || at.After(turns[i-1].last)) {
			return t
		}
	}
	return nil
}

func (d *DB) surfacedEvidence(userSession string) ([]surfacedEvidence, error) {
	rows, err := d.db.Query(`SELECT DISTINCT e.event_id,e.session_id,e.vault_path,e.timestamp,json_extract(r.value,'$.note_id'),s.caller
 FROM events e JOIN sessions s ON s.session_id=e.session_id, json_each(`+actualResultsSQL+`) r
 WHERE s.user_session_id=? AND s.caller LIKE ? AND e.event_type='ask'`, userSession, hookCallerPattern)
	if err != nil {
		return nil, fmt.Errorf("query surfaced evidence: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []surfacedEvidence
	for rows.Next() {
		var s surfacedEvidence
		var ts string
		if err := rows.Scan(&s.event, &s.session, &s.vault, &ts, &s.note, &s.caller); err != nil {
			return nil, fmt.Errorf("scan surfaced evidence: %w", err)
		}
		at, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil || s.note == "" {
			continue
		}
		s.at = at
		out = append(out, s)
	}
	return out, rows.Err()
}

func (d *DB) openedEvidence(userSession string) ([]openedEvidence, error) {
	rows, err := d.db.Query(`SELECT e.vault_path,e.timestamp,json_extract(e.event_data,'$.note_id') FROM events e
 JOIN sessions s ON s.session_id=e.session_id
 WHERE s.user_session_id=? AND e.event_type='note_access' AND json_extract(e.event_data,'$.source')=?`, userSession, string(AccessSourceRead))
	if err != nil {
		return nil, fmt.Errorf("query opened evidence: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []openedEvidence
	for rows.Next() {
		var r openedEvidence
		var ts string
		if err := rows.Scan(&r.vault, &ts, &r.note); err != nil {
			return nil, fmt.Errorf("scan opened evidence: %w", err)
		}
		at, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			continue
		}
		r.at = at
		out = append(out, r)
	}
	return out, rows.Err()
}
