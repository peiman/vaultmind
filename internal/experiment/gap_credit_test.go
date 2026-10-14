package experiment_test

import (
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGapClosingUse_CountsOnlyGapClosingCites(t *testing.T) {
	db := openTestExpDB(t)
	at := time.Now().UTC().Add(-time.Minute)
	sid := evidenceSession(t, db, "credit-session", "vaultmind-userprompt-hook", "/vault")
	hits := []experiment.RetrievalHit{
		{NoteID: "gap-note"},
		{NoteID: "plain"},
		{NoteID: "gap-surfaced"},
		{NoteID: "opened-gap"},
	}
	surface := evidenceEvent(t, db, sid, experiment.EventAsk, "/vault", at, experiment.BuildAskEventData(experiment.AskEventParams{
		RetrievalMode: "keyword", TopHits: hits, BodyDelivered: true,
	}))
	for _, note := range []string{"gap-note", "plain", "opened-gap"} {
		evidenceEvent(t, db, sid, experiment.EventUseEvidence, "/vault", at.Add(time.Second), map[string]any{
			"note_id": note, "evidence": "cited", "surfaced_event_id": surface,
		})
	}
	readSID := evidenceSession(t, db, "credit-session", "cli", "/vault")
	evidenceEvent(t, db, readSID, experiment.EventNoteAccess, "/vault", at.Add(3*time.Second), map[string]any{
		"note_id": "opened-gap", "source": experiment.AccessSourceRead,
	})

	surfaced, cited, err := db.GapClosingUse(7, []string{"gap-note", "gap-surfaced", "opened-gap"})
	require.NoError(t, err)
	assert.Equal(t, 3, surfaced)
	assert.Equal(t, 1, cited, "a plain cite is not gap-closing, and an opened note is not a cite")

	none, alsoNone, err := db.GapClosingUse(7, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, none)
	assert.Equal(t, 0, alsoNone)

	_, _, err = db.GapClosingUse(0, []string{"gap-note"})
	require.Error(t, err)
}
