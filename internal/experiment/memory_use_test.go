package experiment_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// injectAt writes an ask event shaped like a hook's, surfacing noteID at ts.
// Built through the real payload builder so the test breaks if the shape the
// meter reads ever diverges from the shape ask writes.
func injectAt(t *testing.T, db *experiment.DB, caller, noteID string, ts time.Time, bodyDelivered bool) {
	t.Helper()
	sid := startCallerSession(t, db, caller)
	data := experiment.BuildAskEventData(experiment.AskEventParams{
		RetrievalMode: "hybrid",
		TopHits:       []experiment.RetrievalHit{{NoteID: noteID, Rank: 1, Score: 0.02}},
		BodyDelivered: bodyDelivered,
	})
	raw, err := json.Marshal(data)
	require.NoError(t, err)
	_, err = db.Exec(
		`INSERT INTO events (event_id, session_id, event_type, timestamp, vault_path, query_text, event_data)
		 VALUES (?, ?, 'ask', ?, '/vault', 'q', ?)`,
		fmt.Sprintf("evt-%s-%d", noteID, ts.UnixNano()), sid, ts.UTC().Format(time.RFC3339), string(raw))
	require.NoError(t, err)
}

func accessAt(t *testing.T, db *experiment.DB, noteID string, ts time.Time) {
	t.Helper()
	accessWithSource(t, db, noteID, ts, experiment.AccessSourceRead)
}

// accessWithSource writes a note_access with an explicit source, because the
// source is what separates "the agent opened this" from "a query returned it".
// It must actually USE the source it is handed: an earlier version took the
// parameter and wrote note_get regardless, which made the re-surfacing tests
// pass against a meter that could not tell the two apart.
func accessWithSource(t *testing.T, db *experiment.DB, noteID string, ts time.Time, source string) {
	t.Helper()
	sid := startCallerSession(t, db, "cli")
	_, err := db.Exec(
		`INSERT INTO events (event_id, session_id, event_type, timestamp, vault_path, event_data)
		 VALUES (?, ?, 'note_access', ?, '/vault', ?)`,
		fmt.Sprintf("acc-%s-%s-%d", noteID, source, ts.UnixNano()), sid, ts.UTC().Format(time.RFC3339),
		fmt.Sprintf(`{"note_id":%q,"source":%q}`, noteID, source))
	require.NoError(t, err)
}

func startCallerSession(t *testing.T, db *experiment.DB, caller string) string {
	t.Helper()
	sid, err := db.StartSessionWithCaller("/vault", caller, map[string]any{"user": "u", "host": "h"})
	require.NoError(t, err)
	return sid
}

// The measurement that matters: surfaced, then opened.
func TestMemoryUse_CountsAnAccessInsideTheWindow(t *testing.T) {
	db := openTestExpDB(t)
	now := time.Now().UTC()
	injectAt(t, db, "vaultmind-reach-hook", "note-a", now.Add(-2*time.Hour), false)
	accessAt(t, db, "note-a", now.Add(-2*time.Hour).Add(5*time.Minute))

	use, err := db.MemoryUseSince(7)
	require.NoError(t, err)
	assert.Equal(t, 1, use.Injections)
	assert.Equal(t, 1, use.Consumed)
	assert.InDelta(t, 1.0, use.ConsumedRate(), 0.001)
}

// Outside the window is a different working turn. Counting it would let any
// note read once a day appear permanently "consumed", and the rate would be
// meaningless in the flattering direction.
func TestMemoryUse_IgnoresAnAccessOutsideTheWindow(t *testing.T) {
	db := openTestExpDB(t)
	now := time.Now().UTC()
	injectAt(t, db, "vaultmind-reach-hook", "note-a", now.Add(-3*time.Hour), false)
	accessAt(t, db, "note-a", now.Add(-3*time.Hour).Add(45*time.Minute))

	use, err := db.MemoryUseSince(7)
	require.NoError(t, err)
	assert.Equal(t, 1, use.Injections)
	assert.Equal(t, 0, use.Consumed)
}

// An access BEFORE its injection is not caused by it.
func TestMemoryUse_IgnoresAnAccessThatPrecedesTheInjection(t *testing.T) {
	db := openTestExpDB(t)
	now := time.Now().UTC()
	accessAt(t, db, "note-a", now.Add(-2*time.Hour))
	injectAt(t, db, "vaultmind-reach-hook", "note-a", now.Add(-2*time.Hour).Add(10*time.Minute), false)

	use, err := db.MemoryUseSince(7)
	require.NoError(t, err)
	assert.Equal(t, 1, use.Injections)
	assert.Equal(t, 0, use.Consumed)
}

// A CLI `ask` is not an injection: the caller already decided to look. Counting
// it would measure the intent the hooks exist to create.
func TestMemoryUse_ExcludesDirectCLIQueries(t *testing.T) {
	db := openTestExpDB(t)
	now := time.Now().UTC()
	injectAt(t, db, "cli", "note-a", now.Add(-1*time.Hour), false)
	accessAt(t, db, "note-a", now.Add(-1*time.Hour).Add(time.Minute))

	use, err := db.MemoryUseSince(7)
	require.NoError(t, err)
	assert.Equal(t, 0, use.Injections, "only hook-driven surfacing counts")
}

// The recall hook (every prompt) and the reach hook (decision points) are
// different bets and need separate verdicts — an aggregate would let a good one
// hide inside a bad one.
func TestMemoryUse_SplitsByHook(t *testing.T) {
	db := openTestExpDB(t)
	now := time.Now().UTC()
	injectAt(t, db, "vaultmind-reach-hook", "note-a", now.Add(-time.Hour), false)
	accessAt(t, db, "note-a", now.Add(-time.Hour).Add(time.Minute))
	injectAt(t, db, "vaultmind-userprompt-hook", "note-b", now.Add(-time.Hour), false)

	use, err := db.MemoryUseSince(7)
	require.NoError(t, err)
	require.Len(t, use.PerCaller, 2)

	byCaller := map[string]experiment.CallerUse{}
	for _, c := range use.PerCaller {
		byCaller[c.Caller] = c
	}
	assert.Equal(t, 1, byCaller["vaultmind-reach-hook"].Consumed)
	assert.Equal(t, 0, byCaller["vaultmind-userprompt-hook"].Consumed)
}

// An empty log must read as "nothing measured", never as a division by zero or
// a flattering 100%.
func TestMemoryUse_EmptyLogIsZeroNotAnError(t *testing.T) {
	db := openTestExpDB(t)
	use, err := db.MemoryUseSince(7)
	require.NoError(t, err)
	assert.Equal(t, 0, use.Injections)
	assert.Equal(t, 0, use.Consumed)
	assert.InDelta(t, 0.0, use.ConsumedRate(), 0.001)
}

func TestMemoryUse_RejectsANonPositiveWindow(t *testing.T) {
	db := openTestExpDB(t)
	_, err := db.MemoryUseSince(0)
	assert.Error(t, err, "a zero-day window would silently report zero of everything")
}

// THE test this meter needed and did not have.
//
// note_access is logged with source="ask" every time a note is RETURNED by a
// query. The reach hook shows the same five notes over and over — 98% repeats —
// so each repeat logs an access against a note the previous injection had
// surfaced. Counting those made re-showing a note look like reading it, and
// turned a real rate of 0.8% into 27% on live data.
//
// A surfacing is not a consumption. That is the whole distinction the meter
// exists to make, and without this test the meter measured its own noise.
func TestMemoryUse_ASecondSurfacingIsNotAConsumption(t *testing.T) {
	db := openTestExpDB(t)
	now := time.Now().UTC()

	injectAt(t, db, "vaultmind-reach-hook", "note-a", now.Add(-2*time.Hour), false)
	// The same note surfaced again two minutes later by another hook query.
	accessWithSource(t, db, "note-a", now.Add(-2*time.Hour).Add(2*time.Minute), "ask")

	use, err := db.MemoryUseSince(7)
	require.NoError(t, err)
	assert.Equal(t, 1, use.Injections)
	assert.Equal(t, 0, use.Consumed,
		"a note re-surfaced by a later query was shown again, not read — counting it "+
			"makes the repeat rate masquerade as engagement")
}

// A note pulled in as a graph neighbour was likewise surfaced, not read.
func TestMemoryUse_ANeighbourExpansionIsNotAConsumption(t *testing.T) {
	db := openTestExpDB(t)
	now := time.Now().UTC()

	injectAt(t, db, "vaultmind-reach-hook", "note-a", now.Add(-time.Hour), false)
	accessWithSource(t, db, "note-a", now.Add(-time.Hour).Add(time.Minute), "neighbors")

	use, err := db.MemoryUseSince(7)
	require.NoError(t, err)
	assert.Equal(t, 0, use.Consumed)
}

// And the positive case stays positive: an explicit fetch of the note's text
// is what consumption means.
func TestMemoryUse_AnExplicitReadIsAConsumption(t *testing.T) {
	db := openTestExpDB(t)
	now := time.Now().UTC()

	injectAt(t, db, "vaultmind-reach-hook", "note-a", now.Add(-time.Hour), false)
	accessWithSource(t, db, "note-a", now.Add(-time.Hour).Add(time.Minute), experiment.AccessSourceRead)

	use, err := db.MemoryUseSince(7)
	require.NoError(t, err)
	assert.Equal(t, 1, use.Consumed)
}

// The bug this test exists for, and the reason the meter was wrong on its first
// real measurement: note_access is logged with source="ask" whenever a note is
// RETURNED by a query. So a second hook injection of the same note logs an
// access against the first one, and counting those turns repetition into
// "consumption". With reach-hook payloads repeating 98% of the time, that took
// the measured rate from 0.8% to 27% — a number I reported before checking what
// the events meant.
//
// Only an explicit fetch counts.
func TestMemoryUse_ASecondInjectionIsNotConsumption(t *testing.T) {
	db := openTestExpDB(t)
	now := time.Now().UTC()

	// Surfaced by the reach hook...
	injectAt(t, db, "vaultmind-reach-hook", "note-a", now.Add(-time.Hour), false)
	// ...then surfaced AGAIN two minutes later, which logs source="ask".
	accessWithSource(t, db, "note-a", now.Add(-time.Hour).Add(2*time.Minute), "ask")

	use, err := db.MemoryUseSince(7)
	require.NoError(t, err)
	assert.Equal(t, 1, use.Injections)
	assert.Equal(t, 0, use.Consumed,
		"a note surfaced twice was shown twice, not read once")
}

// Graph neighbours are surfacing too — the note was pulled in as context, not
// opened.
func TestMemoryUse_ANeighbourExpansionIsNotConsumption(t *testing.T) {
	db := openTestExpDB(t)
	now := time.Now().UTC()
	injectAt(t, db, "vaultmind-reach-hook", "note-a", now.Add(-time.Hour), false)
	accessWithSource(t, db, "note-a", now.Add(-time.Hour).Add(time.Minute), "neighbors")

	use, err := db.MemoryUseSince(7)
	require.NoError(t, err)
	assert.Equal(t, 0, use.Consumed)
}

// And the positive case, so the filter cannot be "count nothing".
func TestMemoryUse_AnExplicitFetchIsConsumption(t *testing.T) {
	db := openTestExpDB(t)
	now := time.Now().UTC()
	injectAt(t, db, "vaultmind-reach-hook", "note-a", now.Add(-time.Hour), false)
	accessWithSource(t, db, "note-a", now.Add(-time.Hour).Add(time.Minute), experiment.AccessSourceRead)

	use, err := db.MemoryUseSince(7)
	require.NoError(t, err)
	assert.Equal(t, 1, use.Consumed)
}
