package cmd

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// logAskExperiment must log a note_access event with source="ask" for the
// top hit when retrieval succeeded. Closes the gap described in issue #18:
// `ask` is the primary user-facing read path and its accesses were the only
// ones missing from the access-history signal spreading activation uses.
func TestLogAskExperiment_LogsTopHitNoteAccess(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	session := &experiment.Session{DB: db, ID: sid}

	cmd := &cobra.Command{}
	cmd.SetContext(experiment.WithSession(context.Background(), session))

	result := &query.AskResult{
		Query:   "who am I",
		TopHits: []retrieval.ScoredResult{{ID: "identity-who-i-am", Score: 0.065}},
	}

	logAskExperiment(cmd, "who am I", "/vault", "hybrid", result, nil, false)

	ids, err := db.AccessedNoteIDs()
	require.NoError(t, err)
	assert.Contains(t, ids, "identity-who-i-am",
		"ask must log note_access for its top hit (issue #18)")

	var source string
	err = db.QueryRow(
		`SELECT json_extract(event_data,'$.source')
		 FROM events
		 WHERE event_type = 'note_access'
		   AND json_extract(event_data,'$.note_id') = ?`,
		"identity-who-i-am",
	).Scan(&source)
	require.NoError(t, err)
	assert.Equal(t, "ask", source, "source field must distinguish ask from recall/note_get")
}

// logAskExperiment must NOT log note_access when retrieval errored — a
// failed query has no trustworthy "top hit" to record as accessed.
func TestLogAskExperiment_NoNoteAccessOnRetrievalError(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	session := &experiment.Session{DB: db, ID: sid}

	cmd := &cobra.Command{}
	cmd.SetContext(experiment.WithSession(context.Background(), session))

	result := &query.AskResult{
		Query:   "broken",
		TopHits: []retrieval.ScoredResult{{ID: "stale-hit", Score: 0.01}},
	}
	logAskExperiment(cmd, "broken", "/vault", "hybrid", result, assert.AnError, false)

	ids, err := db.AccessedNoteIDs()
	require.NoError(t, err)
	assert.NotContains(t, ids, "stale-hit",
		"note_access must not be logged on retrieval error")
}

// logAskExperiment must NOT log note_access when there are no hits —
// there's no top hit to record.
func TestLogAskExperiment_NoNoteAccessOnEmptyHits(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	session := &experiment.Session{DB: db, ID: sid}

	cmd := &cobra.Command{}
	cmd.SetContext(experiment.WithSession(context.Background(), session))

	result := &query.AskResult{Query: "no-hits", TopHits: nil}
	logAskExperiment(cmd, "no-hits", "/vault", "hybrid", result, nil, false)

	ids, err := db.AccessedNoteIDs()
	require.NoError(t, err)
	assert.Empty(t, ids, "no note_access event should be logged when ask returned no hits")
}

// logAskExperiment must NOT reinforce the top hit when the recall was
// suppressed (recall-floor no_match). The whole point of the floor is that an
// off-domain prompt neither injects noise nor pollutes the activation signal.
func TestLogAskExperiment_NoNoteAccessWhenSuppressed(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	session := &experiment.Session{DB: db, ID: sid}

	cmd := &cobra.Command{}
	cmd.SetContext(experiment.WithSession(context.Background(), session))

	result := &query.AskResult{
		Query:   "best recipe for sourdough bread",
		TopHits: []retrieval.ScoredResult{{ID: "reference-current-context", Score: 0.02}},
	}
	logAskExperiment(cmd, "best recipe for sourdough bread", "/vault", "hybrid", result, nil, true)

	ids, err := db.AccessedNoteIDs()
	require.NoError(t, err)
	assert.NotContains(t, ids, "reference-current-context",
		"a suppressed recall-floor no_match must not reinforce the irrelevant top hit")
}
