package experiment_test

import (
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
)

// TestScoreFromData_CountWinsOnlyOnceRecencyFades pins the property that three
// scorer tests quietly depended on and one of them finally failed on in CI:
// "accessed more often ranks higher" is a LONG-RUN property of ACT-R, not an
// instantaneous one.
//
// Retrieval strength is ln(sum(t^-d)) — a power law that goes to infinity as an
// access approaches the present. So immediately after the events, a single very
// recent access outranks five slightly older ones; the count term (ln(1+n), which
// never decays) only takes over once elapsed time is large enough that the
// recency terms have converged.
//
// That is correct behaviour, not a bug — but it means a test that logs 5 accesses
// to one note, 1 to another, and then scores at time.Now() is asserting the
// long-run ordering while measuring in the instantaneous regime. Event timestamps
// are stored at second resolution while `now` carries nanoseconds, so which
// regime the test lands in depends on whether the two LogEvent calls happened to
// straddle a second boundary. On a loaded CI runner they did:
//
//	"4.1377065629080665" is not greater than "4.3734399781196975"
//
// The fix is for those tests to score at an explicit instant well after the
// accesses (ComputeBatchScoresAt). This test is why that is the right fix rather
// than a loosened assertion.
func TestScoreFromData_CountWinsOnlyOnceRecencyFades(t *testing.T) {
	base := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	ids := []string{"frequent", "recent"}
	accessMap := map[string][]time.Time{
		"frequent": {base, base, base, base, base},
		"recent":   {base.Add(1990 * time.Millisecond)},
	}
	// gamma=1.0 (wall-clock) with no session windows: elapsed is plain elapsed,
	// so the arithmetic below is the ACT-R power law and nothing else.
	params := experiment.DefaultActivationParams(1.0)

	// Two seconds in: "recent" is 10ms old and "frequent" is 2s old, a 200x
	// ratio. The single access wins despite being outnumbered five to one.
	atOnce, _ := experiment.ScoreFromData(ids, accessMap, nil, base.Add(2*time.Second), params, nil)
	assert.Greater(t, atOnce["recent"], atOnce["frequent"],
		"immediately after the accesses, recency dominates count")

	// An hour later both are ~3600s old, the recency terms have converged, and
	// the never-decaying count term decides. This is the regime the scorer tests
	// mean to assert, and the only one where their assertion is stable.
	atHour, _ := experiment.ScoreFromData(ids, accessMap, nil, base.Add(time.Hour), params, nil)
	assert.Greater(t, atHour["frequent"], atHour["recent"],
		"once recency has faded, the note accessed more often ranks higher")
}
