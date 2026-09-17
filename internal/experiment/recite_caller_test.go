package experiment_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/require"
)

// The recitation row carries its own caller tag.
//
// The tag belongs on the row `arc recite` actually writes — the
// experiment-ledger event — and NOT as a vault-local access row. `self`
// filters by body_delivered, not caller (migration 008 retired the caller
// proxy), so a vault-local recite row would surface in the hot list whatever
// it was tagged, and 27 arcs every session would bury deliberate reads. The
// tag here is what makes recitations filterable without that cost.
func TestLogNoteAccessEventAs_CarriesTheCallerTag(t *testing.T) {
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	s := &experiment.Session{DB: db, ID: sid}

	_, err = s.LogNoteAccessEventAs("arc-x", experiment.AccessSourceRecite, true, experiment.CallerRecite)
	require.NoError(t, err)

	var source, caller string
	err = db.QueryRow(`SELECT json_extract(event_data,'$.source'), json_extract(event_data,'$.caller')
		FROM events WHERE event_type = ? AND json_extract(event_data,'$.note_id') = ?`,
		experiment.EventNoteAccess, "arc-x").Scan(&source, &caller)
	require.NoError(t, err)

	require.Equal(t, string(experiment.AccessSourceRecite), source)
	require.Equal(t, experiment.CallerRecite, caller,
		"the tag is what makes recitations filterable in the ledger")
}

// The existing logger keeps its shape — no caller key appears where callers
// never set one, so nothing downstream starts seeing an empty string.
func TestLogNoteAccessEvent_UnchangedWhenNoCallerGiven(t *testing.T) {
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	s := &experiment.Session{DB: db, ID: sid}

	_, err = s.LogNoteAccessEvent("note-y", experiment.AccessSourceRead, true)
	require.NoError(t, err)

	var caller *string
	err = db.QueryRow(`SELECT json_extract(event_data,'$.caller')
		FROM events WHERE event_type = ? AND json_extract(event_data,'$.note_id') = ?`,
		experiment.EventNoteAccess, "note-y").Scan(&caller)
	require.NoError(t, err)
	require.Nil(t, caller, "an unset caller must stay absent, not become an empty string")
}
