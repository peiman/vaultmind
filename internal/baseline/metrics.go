// Package baseline provides pre-curated golden-query regression metrics for
// VaultMind's retrievers. Baselines are committed snapshots of Hit@K and
// MRR against a stable query fixture. The point is not absolute quality
// (that's research-grade evaluation); it's a tripwire that fires when a
// change degrades retrieval on a fixed input — the "you cannot improve
// what you cannot measure" principle applied to a single dimension.
package baseline

// HitAtK reports 1.0 if any expected ID appears within the first k results,
// else 0.0. A float return type keeps aggregation (mean across queries)
// arithmetically simple.
//
// Semantics: strict-intersection — any expected in top-k counts. Not
// all-of-expected, not strict-order. Partial-recall wins are the point of
// keyword/hybrid retrieval and we don't want the metric to punish them.
//
// k larger than len(results) scans everything that exists (no panic). An
// empty expected set returns 0 rather than NaN so downstream means stay
// well-defined.
func HitAtK(results, expected []string, k int) float64 {
	if len(expected) == 0 || len(results) == 0 {
		return 0
	}
	seen := make(map[string]bool, len(expected))
	for _, e := range expected {
		seen[e] = true
	}
	limit := min(k, len(results))
	for i := range limit {
		if seen[results[i]] {
			return 1
		}
	}
	return 0
}

// ReciprocalRank reports 1/rank of the best (lowest-rank) expected ID
// within results, or 0 if no expected ID appears at all. This is the
// per-query contribution to MRR; the mean across queries is taken at the
// aggregate layer.
//
// "Best rank wins" — if multiple expected IDs are present, we take the
// minimum rank. Using first-declared expected ID would silently tie the
// metric to the *order* of the expected set in the fixture, which is a
// trap: reshuffling the fixture (for readability, say) would then move
// the metric.
func ReciprocalRank(results, expected []string) float64 {
	if len(results) == 0 || len(expected) == 0 {
		return 0
	}
	seen := make(map[string]bool, len(expected))
	for _, e := range expected {
		seen[e] = true
	}
	for i, r := range results {
		if seen[r] {
			return 1.0 / float64(i+1)
		}
	}
	return 0
}
