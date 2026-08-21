package experiment

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMemoryUseSQL_MaterialisesTheAccessCTE guards the one keyword that decides
// whether `doctor` is a health command or an unusable one.
//
// The EXISTS in this query is CORRELATED — it references inj.note_id — and `acc`
// json_extract's over every note_access event in the window. Without the
// MATERIALIZED hint SQLite re-evaluates `acc` once per outer row: six thousand
// injections times the whole access log.
//
// Measured on a live log, in C SQLite (the pure-Go driver here is slower):
//
//	as-is           188.73s  -> 6505 | 44
//	MATERIALIZED      0.068s -> 6535 | 44   (same answer)
//
// The cost tracks the EVENT LOG, not the vault, so it grows with use for every
// adopter — and it rides in the SessionStart hook, which is the worst possible
// place for a tax that compounds.
//
// This asserts the QUERY PLAN, not the clock. A wall-clock guard was written
// first and rejected: on a shared machine three separate timeouts today were
// contention rather than code, and a timing test that goes red under load is a
// test somebody deletes. The plan is deterministic at any load. Correctness is
// covered by the existing TestMemoryUse_* cases; this covers only the shape.
func TestMemoryUseSQL_MaterialisesTheAccessCTE(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "exp.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	for _, tc := range []struct {
		name string
		sql  string
	}{
		{"MemoryUseSince", memoryUseSQL},
		{"memoryUsePerCaller", memoryUsePerCallerSQL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Contains(t, tc.sql, "acc AS MATERIALIZED",
				"the hint is the fix; without it this query is quadratic in usage")

			rows, err := db.db.Query("EXPLAIN QUERY PLAN "+tc.sql,
				hookCallerPattern, "-7 days", string(AccessSourceRead), "-7 days", "+30 minutes")
			require.NoError(t, err)
			defer func() { _ = rows.Close() }()

			var plan strings.Builder
			for rows.Next() {
				var id, parent, notused int
				var detail string
				require.NoError(t, rows.Scan(&id, &parent, &notused, &detail))
				plan.WriteString(detail)
				plan.WriteString("\n")
			}
			require.NoError(t, rows.Err())

			// SQLite reports a materialised CTE explicitly. Its absence means the
			// planner chose to re-evaluate acc per outer row — the exact regression.
			require.Contains(t, strings.ToUpper(plan.String()), "MATERIALIZE",
				"query plan does not materialise acc:\n%s", plan.String())
		})
	}
}
