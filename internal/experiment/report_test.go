package experiment_test

import (
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedOutcome inserts an outcome row directly for test setup.
func seedOutcome(t *testing.T, db *experiment.DB, outcomeID, eventID, noteID, variant string, rank int, sessionID string) {
	t.Helper()
	accessedAt := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO outcomes (outcome_id, event_id, note_id, variant, rank, accessed_at, session_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		outcomeID, eventID, noteID, variant, rank, accessedAt, sessionID,
	)
	require.NoError(t, err)
}

// seedFullExperiment seeds a complete experiment scenario:
//   - 2 sessions (s1, s2)
//   - 3 search events (e1 in s1, e2 in s1, e3 in s2)
//   - Outcomes: e1→n1 (v-a rank 1, v-b rank 3), e3→n6 (v-a rank 2, v-b rank 1). e2 has no outcome.
func seedFullExperiment(t *testing.T, db *experiment.DB) {
	t.Helper()

	seedSession(t, db, "s1", "2024-01-01T10:00:00Z")
	seedSession(t, db, "s2", "2024-01-01T11:00:00Z")

	emptyData := map[string]any{"variants": map[string]any{}}
	seedSearchEvent(t, db, "s1", "e1", emptyData)
	seedSearchEvent(t, db, "s1", "e2", emptyData)
	seedSearchEvent(t, db, "s2", "e3", emptyData)

	// e1: v-a rank 1, v-b rank 3
	seedOutcome(t, db, "o1", "e1", "n1", "v-a", 1, "s1")
	seedOutcome(t, db, "o2", "e1", "n1", "v-b", 3, "s1")

	// e3: v-a rank 2, v-b rank 1
	seedOutcome(t, db, "o3", "e3", "n6", "v-a", 2, "s2")
	seedOutcome(t, db, "o4", "e3", "n6", "v-b", 1, "s2")
}

func TestReport_Basic(t *testing.T) {
	db := openTestExpDB(t)
	seedFullExperiment(t, db)

	result, err := db.Report([]string{"v-a", "v-b"}, 5)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 2, result.SessionCount)
	assert.Equal(t, 3, result.EventCount)
	assert.Equal(t, 4, result.OutcomeCount)
	assert.Equal(t, 5, result.K)
	assert.Len(t, result.Variants, 2)
	assert.Contains(t, result.Variants, "v-a")
	assert.Contains(t, result.Variants, "v-b")
}

func TestReport_HitAtK(t *testing.T) {
	db := openTestExpDB(t)
	seedFullExperiment(t, db)

	result, err := db.Report([]string{"v-a", "v-b"}, 5)
	require.NoError(t, err)

	// v-a: e1 rank 1 (hit), e2 no outcome (miss), e3 rank 2 (hit) → 2/3 ≈ 0.667
	assert.InDelta(t, 0.667, result.Variants["v-a"].HitAtK, 0.01)
	// v-b: e1 rank 3 (hit, ≤5), e2 no outcome (miss), e3 rank 1 (hit) → 2/3 ≈ 0.667
	assert.InDelta(t, 0.667, result.Variants["v-b"].HitAtK, 0.01)
}

func TestReport_MRR(t *testing.T) {
	db := openTestExpDB(t)
	seedFullExperiment(t, db)

	result, err := db.Report([]string{"v-a", "v-b"}, 5)
	require.NoError(t, err)

	// v-a: (1/1 + 0 + 1/2) / 3 = 1.5/3 = 0.5
	assert.InDelta(t, 0.5, result.Variants["v-a"].MRR, 0.01)
	// v-b: (1/3 + 0 + 1/1) / 3 = (0.333 + 1) / 3 ≈ 0.444
	assert.InDelta(t, 0.444, result.Variants["v-b"].MRR, 0.01)
}

func TestReport_HitAtK_Respects_K(t *testing.T) {
	db := openTestExpDB(t)
	seedFullExperiment(t, db)

	result, err := db.Report([]string{"v-a", "v-b"}, 1)
	require.NoError(t, err)

	// v-a: e1 rank 1 (≤1 hit), e2 no outcome (miss), e3 rank 2 (>1 miss) → 1/3 ≈ 0.333
	assert.InDelta(t, 0.333, result.Variants["v-a"].HitAtK, 0.01)
	// v-b: e1 rank 3 (>1 miss), e2 no outcome (miss), e3 rank 1 (≤1 hit) → 1/3 ≈ 0.333
	assert.InDelta(t, 0.333, result.Variants["v-b"].HitAtK, 0.01)
}

func TestReport_EmptyDB(t *testing.T) {
	db := openTestExpDB(t)

	result, err := db.Report([]string{"v-a"}, 5)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 0, result.SessionCount)
	assert.Equal(t, 0, result.EventCount)
	assert.Equal(t, 0, result.OutcomeCount)
	assert.Empty(t, result.Variants)
}

func TestDistinctVariants(t *testing.T) {
	db := openTestExpDB(t)
	seedFullExperiment(t, db)

	variants, err := db.DistinctVariants()
	require.NoError(t, err)
	assert.Equal(t, []string{"v-a", "v-b"}, variants)
}

func TestDistinctVariants_EmptyDB(t *testing.T) {
	db := openTestExpDB(t)

	variants, err := db.DistinctVariants()
	require.NoError(t, err)
	assert.Empty(t, variants)
}
