package experiment_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
)

// DefaultSpreadingActivationDelta is the Delta value used when the scorer
// has query-similarities available (hybrid retrieval on). The value 0.2 is
// the research-driven default from the activation arcs. Locking the value
// here makes any change show up in a diff — the "numbers were wrong" arc
// happened because this value leaked between packages without visibility.
func TestDefaultSpreadingActivationDelta_IsLockedAt02(t *testing.T) {
	assert.Equal(t, 0.2, experiment.DefaultSpreadingActivationDelta,
		"DefaultSpreadingActivationDelta must be 0.2 — research-driven default; changing this is a deliberate decision, not a refactor")
}

// DefaultActivationParamsWithSimilarity returns a params set tuned for the
// similarity-available case. Delta is the spreading-activation weight; the
// other params match DefaultActivationParams so callers get a single
// swap-in constructor when they know they'll supply similarities.
func TestDefaultActivationParamsWithSimilarity_SetsOnlyDelta(t *testing.T) {
	baseline := experiment.DefaultActivationParams(0.5)
	withSim := experiment.DefaultActivationParamsWithSimilarity(0.5)

	assert.Equal(t, 0.2, withSim.Delta,
		"WithSimilarity constructor must set Delta to the spreading-activation default")
	// Every other field must match the no-similarity default so callers
	// can swap constructors without cascading changes.
	assert.Equal(t, baseline.Gamma, withSim.Gamma)
	assert.Equal(t, baseline.D, withSim.D)
	assert.Equal(t, baseline.Alpha, withSim.Alpha)
	assert.Equal(t, baseline.Beta, withSim.Beta)
}

// DefaultActivationParams keeps Delta=0.0 — the no-similarity case is the
// backward-compatibility path. Regression guard so the two constructors
// stay distinct.
func TestDefaultActivationParams_DeltaStaysZero(t *testing.T) {
	p := experiment.DefaultActivationParams(0.5)
	assert.Equal(t, 0.0, p.Delta,
		"DefaultActivationParams must keep Delta=0 — callers opt into similarity via the other constructor")
}

// MinElapsedHours is the soft floor preventing log(0) in the retrieval
// strength formula for very-recent accesses. Locked at 1 second (in
// hours). Changing it shifts recent-access scores.
func TestMinElapsedHours_IsOneSecondInHours(t *testing.T) {
	assert.InDelta(t, 1.0/3600.0, experiment.MinElapsedHours, 1e-12,
		"MinElapsedHours must equal 1 second in hours — changing this shifts recent-access retrieval scores")
}
