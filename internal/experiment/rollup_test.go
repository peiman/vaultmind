package experiment_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// VariantPerformance returns one entry per distinct primary_variant
// observed in events, with MRR / Hit@K computed from joined outcomes.
// The denominator on the rate fields is OutcomeCount — events without
// outcomes can't contribute to retrieval-quality measurement.
func TestVariantPerformance_BasicFlow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exp.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	sid, err := db.StartSession("/v")
	require.NoError(t, err)

	// Two events: one under variant "compressed-0.5", one under "wall-clock".
	_, err = db.LogEvent(experiment.Event{
		SessionID:      sid,
		Type:           experiment.EventAsk,
		VaultPath:      "/v",
		PrimaryVariant: "compressed-0.5",
		Data: map[string]any{
			"variants": map[string]any{
				"compressed-0.5": map[string]any{
					"results": []any{map[string]any{"note_id": "n1", "rank": float64(1)}},
				},
			},
		},
	})
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID:      sid,
		Type:           experiment.EventAsk,
		VaultPath:      "/v",
		PrimaryVariant: "wall-clock",
		Data: map[string]any{
			"variants": map[string]any{
				"wall-clock": map[string]any{
					"results": []any{map[string]any{"note_id": "n2", "rank": float64(3)}},
				},
			},
		},
	})
	require.NoError(t, err)

	// Link outcomes: n1 was accessed (rank 1 under compressed-0.5),
	// n2 was accessed (rank 3 under wall-clock).
	_, err = db.LinkOutcomes(sid, "n1", 5)
	require.NoError(t, err)
	_, err = db.LinkOutcomes(sid, "n2", 5)
	require.NoError(t, err)

	stats, err := experiment.VariantPerformance(db)
	require.NoError(t, err)
	require.Len(t, stats, 2)

	c05 := stats["compressed-0.5"]
	require.NotNil(t, c05)
	assert.Equal(t, 1, c05.EventCount)
	assert.Equal(t, 1, c05.OutcomeCount)
	assert.InDelta(t, 1.0, c05.MRR, 1e-9, "rank 1 → MRR 1.0")
	assert.InDelta(t, 1.0, c05.HitAt5, 1e-9, "rank 1 ≤ 5")

	wc := stats["wall-clock"]
	require.NotNil(t, wc)
	assert.Equal(t, 1, wc.EventCount)
	assert.Equal(t, 1, wc.OutcomeCount)
	assert.InDelta(t, 1.0/3.0, wc.MRR, 1e-9, "rank 3 → MRR 0.333…")
	assert.InDelta(t, 1.0, wc.HitAt5, 1e-9, "rank 3 ≤ 5")
	assert.InDelta(t, 1.0, wc.HitAt10, 1e-9, "rank 3 ≤ 10")
}

// Empty DB: VariantPerformance returns empty map, not error. The
// federated payload must be safe to compute even when nothing's been
// logged yet (e.g. just-installed vault on first export attempt).
func TestVariantPerformance_EmptyDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exp.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	stats, err := experiment.VariantPerformance(db)
	require.NoError(t, err)
	assert.Empty(t, stats)
}

// Events with no primary_variant are skipped — the older event-shape
// (events before the activation experiment was wired) shouldn't taint
// the variant-level rollup. This is the regression guard for the
// "events.primary_variant IS NULL" partition.
func TestVariantPerformance_NoPrimaryVariantEventsExcluded(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exp.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	sid, err := db.StartSession("/v")
	require.NoError(t, err)
	// PrimaryVariant intentionally empty — pre-activation shape.
	_, err = db.LogEvent(experiment.Event{
		SessionID: sid,
		Type:      experiment.EventAsk,
		VaultPath: "/v",
		Data:      map[string]any{},
	})
	require.NoError(t, err)

	stats, err := experiment.VariantPerformance(db)
	require.NoError(t, err)
	assert.Empty(t, stats, "events without primary_variant must not appear in variant rollup")
}
