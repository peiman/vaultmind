package query_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The index ledger recorded delivery from its own heuristic —
// `packResult.Target.Body != ""` — which is packHasText, not delivery. The pack
// is ASSEMBLED under --pointers-only and withheld at render, so every row landed
// body_delivered=1 for a command that handed over nothing.
//
// Measured on a real vault before this fix: `ask --pointers-only` printed
// "751 tokens of note text withheld" and wrote 2 rows marked delivered.
//
// It is not hypothetical. load-persona.sh deliberately keeps --pointers-only on
// its second query, so every SessionStart credited that fan-out to `vaultmind
// self` as engagement — readmitting exactly the hook pollution migration 007 was
// built to exclude, through the branch migration 008 added. 008 fixed the
// inversion in one direction and opened it in the other: `self` stopped hiding
// real reads and started claiming reads that never happened.
//
// The comment on RecordNoteAccessDelivered claimed both ledgers answer from one
// AskResult.DeliveredTo value. They did not. This makes that true.
func TestAsk_PointersOnlyRecordsNoDelivery(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "idx.db")
	db := testvault.OpenSharedDB(t, testVaultPath, dbPath)
	t.Cleanup(func() { _ = db.Close() })
	resolver := graph.NewResolver(db)

	_, err := query.Ask(t.Context(), &query.FTSRetriever{DB: db}, resolver, db, query.AskConfig{
		Query:        "spreading activation",
		Budget:       900,
		MaxItems:     3,
		SearchLimit:  5,
		PointersOnly: true,
	})
	require.NoError(t, err)

	var delivered int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM note_accesses WHERE body_delivered = 1`).Scan(&delivered))

	assert.Zero(t, delivered,
		"--pointers-only hands over no text; marking %d rows delivered feeds `self` and the "+
			"activation scorer reads that never happened", delivered)
}

// The mirror: a normal ask that DOES render bodies must still record them, or
// this fix trades a false positive for a false negative and `self` goes blind
// again.
func TestAsk_DeliveringAskRecordsDelivery(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "idx.db")
	db := testvault.OpenSharedDB(t, testVaultPath, dbPath)
	t.Cleanup(func() { _ = db.Close() })
	resolver := graph.NewResolver(db)

	result, err := query.Ask(t.Context(), &query.FTSRetriever{DB: db}, resolver, db, query.AskConfig{
		Query:       "spreading activation",
		Budget:      8192,
		MaxItems:    3,
		SearchLimit: 5,
	})
	require.NoError(t, err)
	require.NotNil(t, result.Context)

	var delivered int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM note_accesses WHERE body_delivered = 1`).Scan(&delivered))

	assert.Positive(t, delivered, "an ask that rendered bodies must record them as delivered")
}

// A note whose body was NOT packed is not a delivery even on a delivering ask —
// per-item text decides, not the whole-result verdict.
//
// Budget 500 is chosen, not arbitrary: it is where this fixture produces BOTH
// kinds of item. The first draft of this test used 8192, where every item has
// text — so the loop body executed zero times and the test asserted nothing at
// all. It passed, and it could never have failed. Swept:
//
//	budget  200 -> 0 with text, 4 without
//	budget  500 -> 3 with text, 1 without   <- both branches exercised
//	budget 8192 -> 4 with text, 0 without   <- the vacuous one
//
// The require() below is the guard that matters more than the budget: if the
// fixture ever drifts so that one kind disappears, this fails loudly instead of
// quietly going hollow again.
func TestAsk_ItemsWithoutTextAreNotDeliveries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "idx.db")
	db := testvault.OpenSharedDB(t, testVaultPath, dbPath)
	t.Cleanup(func() { _ = db.Close() })
	resolver := graph.NewResolver(db)

	result, err := query.Ask(t.Context(), &query.FTSRetriever{DB: db}, resolver, db, query.AskConfig{
		Query: "spreading activation", Budget: 500, MaxItems: 5, SearchLimit: 5,
	})
	require.NoError(t, err)
	require.NotNil(t, result.Context)

	var withText, withoutText int
	for _, item := range result.Context.Context {
		if item.Body != "" {
			withText++
			continue
		}
		withoutText++

		var n int
		require.NoError(t, db.QueryRow(
			`SELECT COUNT(*) FROM note_accesses WHERE note_id = ? AND body_delivered = 1`,
			item.ID).Scan(&n))
		assert.Zero(t, n, "item %s carried no text; it is not a delivery", item.ID)
	}

	require.Positive(t, withoutText,
		"no item lacked text, so the assertion above never ran — this test would pass against "+
			"any implementation, which is the defect it exists to catch")
	require.Positive(t, withText,
		"every item lacked text, so the mixed case this test is about was never exercised")
}
