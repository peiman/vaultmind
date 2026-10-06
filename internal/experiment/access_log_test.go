package experiment

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// scanAccessedNoteIDsReference is AccessedNoteIDs as it was before the
// access log was kept: a fresh scan, verbatim.
func scanAccessedNoteIDsReference(t *testing.T, d *DB) []string {
	rows, err := d.db.Query(`SELECT event_data FROM events WHERE event_type = ?`, EventNoteAccess)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	seen := make(map[string]bool)
	var ids []string
	for rows.Next() {
		var dataJSON string
		if err := rows.Scan(&dataJSON); err != nil {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
			continue
		}
		noteID, ok := data["note_id"].(string)
		if !ok || seen[noteID] {
			continue
		}
		if !activationSignalFrom(data) {
			continue
		}
		seen[noteID] = true
		ids = append(ids, noteID)
	}
	require.NoError(t, rows.Err())
	return ids
}

// scanBatchNoteAccessTimesReference is BatchNoteAccessTimes as it was, verbatim.
func scanBatchNoteAccessTimesReference(t *testing.T, d *DB, noteIDs []string) map[string][]time.Time {
	result := make(map[string][]time.Time, len(noteIDs))
	for _, id := range noteIDs {
		result[id] = nil
	}
	if len(noteIDs) == 0 {
		return result
	}
	wanted := make(map[string]bool, len(noteIDs))
	for _, id := range noteIDs {
		wanted[id] = true
	}
	rows, err := d.db.Query(`SELECT timestamp, event_data FROM events WHERE event_type = ? ORDER BY timestamp ASC`, EventNoteAccess)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var ts, dataJSON string
		if err := rows.Scan(&ts, &dataJSON); err != nil {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
			continue
		}
		noteID, ok := data["note_id"].(string)
		if !ok || !wanted[noteID] {
			continue
		}
		if !activationSignalFrom(data) {
			continue
		}
		tm, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		result[noteID] = append(result[noteID], tm)
	}
	require.NoError(t, rows.Err())
	return result
}

func insertRawEvent(t *testing.T, d *DB, sid, typ, ts, data string) {
	t.Helper()
	_, err := d.db.Exec(`INSERT INTO events (event_id, session_id, event_type, timestamp, vault_path, event_data) VALUES (?, ?, ?, ?, ?, ?)`,
		newUUID(), sid, typ, ts, "/v", data)
	require.NoError(t, err)
}

// randomEvent returns an event of a kind the access log must treat exactly as
// a fresh scan does: counted, filtered out, malformed, or another type.
func randomEvent(r *rand.Rand, base time.Time) (typ, ts, data string) {
	note := fmt.Sprintf("n%d", r.Intn(12))
	ts = base.Add(time.Duration(r.Intn(400)) * time.Second).Format(time.RFC3339) // ties and backdating
	switch r.Intn(10) {
	case 0:
		return EventNoteAccess, ts, `{"note_id":"` + note + `","source":"ask","body_delivered":false}` // filtered out
	case 1:
		return EventNoteAccess, ts, `{"note_id":"` + note + `","source":"recite"}` // filtered out
	case 2:
		return EventNoteAccess, ts, `{not json`
	case 3:
		return EventNoteAccess, "yesterday", `{"note_id":"` + note + `","source":"ask","body_delivered":true}` // counted id, no time
	case 4:
		return EventNoteAccess, ts, `{"note_id":7,"source":"ask","body_delivered":true}`
	case 5:
		return "search", ts, `{"note_id":"` + note + `","source":"ask","body_delivered":true}`
	default:
		return EventNoteAccess, ts, `{"note_id":"` + note + `","source":"ask","body_delivered":true}`
	}
}

// The kept access log answers exactly as a fresh scan of the events would,
// read after read, however events arrive in between.
func TestAccessLog_EqualsFreshScansAcrossWrites(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "exp.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	sid, err := d.StartSession("/v")
	require.NoError(t, err)
	r := rand.New(rand.NewSource(5))
	base := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)

	for step := 0; step < 120; step++ {
		for i := r.Intn(4); i > 0; i-- {
			typ, ts, data := randomEvent(r, base)
			insertRawEvent(t, d, sid, typ, ts, data)
		}
		ids, err := d.AccessedNoteIDs()
		require.NoError(t, err)
		require.Equal(t, scanAccessedNoteIDsReference(t, d), ids, "step %d", step)

		ask := append([]string{"never-accessed"}, ids...)
		if step%3 == 0 {
			ask = ask[:1+len(ask)/2]
		}
		got, err := d.BatchNoteAccessTimes(ask)
		require.NoError(t, err)
		require.Equal(t, scanBatchNoteAccessTimesReference(t, d, ask), got, "step %d", step)
	}
}

// A caller changing a returned slice does not change what the log holds.
func TestAccessLog_ReturnsCopies(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "exp.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	sid, err := d.StartSession("/v")
	require.NoError(t, err)
	insertRawEvent(t, d, sid, EventNoteAccess, "2026-10-07T09:00:00Z", `{"note_id":"a","source":"ask","body_delivered":true}`)

	first, err := d.BatchNoteAccessTimes([]string{"a"})
	require.NoError(t, err)
	first["a"][0] = time.Time{}
	ids, err := d.AccessedNoteIDs()
	require.NoError(t, err)
	ids[0] = "changed"

	again, err := d.BatchNoteAccessTimes([]string{"a"})
	require.NoError(t, err)
	require.Equal(t, scanBatchNoteAccessTimesReference(t, d, []string{"a"}), again)
	require.Equal(t, []string{"a"}, scanAccessedNoteIDsReference(t, d))
	idsAgain, _ := d.AccessedNoteIDs()
	require.Equal(t, []string{"a"}, idsAgain)
}
