package experiment

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// accessLog is the decoded note_access history of one DB handle. An ask
// scores activation about three times and each score read every access event
// twice, JSON-decoding each one — a cost that grows with every access ever
// recorded. The log decodes each event once; later reads fold in only the
// events inserted since, so the answers stay exactly those of a fresh scan
// even when the ask writes events between its reads.
//
// Events are append-only (nothing deletes or rewrites them), which is what
// makes reading "rowid > last seen" complete.
type accessLog struct {
	mu      sync.Mutex
	lastRow int64
	ids     []string        // first passing event per note, in insertion order
	seen    map[string]bool // ids already in ids
	times   map[string][]accessTime
}

// accessTime keeps the timestamp string beside its parsed time: the scan
// ordered by the string, so the log does too.
type accessTime struct {
	ts string
	t  time.Time
}

func newAccessLog() *accessLog {
	return &accessLog{seen: map[string]bool{}, times: map[string][]accessTime{}}
}

// refresh folds in the note_access events inserted since the last read.
func (l *accessLog) refresh(d *DB) error {
	rows, err := d.db.Query(
		`SELECT rowid, timestamp, event_data FROM events
		 WHERE event_type = ? AND rowid > ? ORDER BY rowid`,
		EventNoteAccess, l.lastRow,
	)
	if err != nil {
		return fmt.Errorf("querying note access events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var row int64
		var ts, dataJSON string
		if err := rows.Scan(&row, &ts, &dataJSON); err != nil {
			continue
		}
		l.lastRow = row
		var data map[string]any
		if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
			continue
		}
		noteID, ok := data["note_id"].(string)
		if !ok || !activationSignalFrom(data) {
			continue
		}
		if !l.seen[noteID] {
			l.seen[noteID] = true
			l.ids = append(l.ids, noteID)
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		l.add(noteID, accessTime{ts: ts, t: t})
	}
	return rows.Err()
}

// add places at in its note's history in timestamp-string order, after any
// equal timestamps. Events nearly always arrive in order, so this is an
// append; a backdated one is inserted where the scan would have put it.
func (l *accessLog) add(noteID string, at accessTime) {
	ts := l.times[noteID]
	i := sort.Search(len(ts), func(i int) bool { return ts[i].ts > at.ts })
	ts = append(ts, accessTime{})
	copy(ts[i+1:], ts[i:])
	ts[i] = at
	l.times[noteID] = ts
}

// withAccessLog brings the handle's access log up to date and runs read on
// it, holding the log's lock for both.
func (d *DB) withAccessLog(read func(*accessLog)) error {
	d.accessOnce.Do(func() { d.access = newAccessLog() })
	l := d.access
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.refresh(d); err != nil {
		return err
	}
	read(l)
	return nil
}
