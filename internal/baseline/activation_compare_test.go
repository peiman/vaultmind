//go:build dev

package baseline_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/peiman/vaultmind/internal/baseline"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/stretchr/testify/require"
)

// TestActivationLaneCompare runs the curated golden-query set through
// BOTH the 4-way RRF (current default) and the 5-way RRF with the
// activation lane appended (slice 5b' opt-in). Side-by-side reports
// answer step (3) of the post-5b' measurement chain in
// reference-current-context: do rankings shift in useful ways for
// known queries?
//
// "Useful" means three things, evaluated qualitatively from the output:
//
//   - Aggregate Hit@5 / MRR don't drop. Activation amplifies recall on
//     accessed-and-relevant notes; if it's drowning out relevant-but-
//     never-accessed notes, that surfaces here as a regression.
//   - Per-query rank-1 changes are interpretable. The expected pattern
//     is that frequently-accessed notes that ALSO match the query move
//     up; recently-accessed-but-irrelevant notes don't push relevant
//     notes off the page.
//   - The "compressed-0.5" gamma (today's primary) shows up as the
//     activation lane's signal source. Future probes may compare gammas.
//
// Set VAULTMIND_ACTIVATION_COMPARE=<vault> to run. The test prints
// reports but does not fail on differences — this is a measurement
// instrument, not a gate. Floor gating belongs to TestLiveBaseline
// once activation goes default-on.
func TestActivationLaneCompare(t *testing.T) {
	name := os.Getenv("VAULTMIND_ACTIVATION_COMPARE")
	if name == "" {
		t.Skip("set VAULTMIND_ACTIVATION_COMPARE=identity|research to run")
	}

	type vaultSpec struct {
		dir     string
		queries string
	}
	vaults := map[string]vaultSpec{
		"identity": {
			dir:     "../../vaultmind-identity",
			queries: "../../test/fixtures/baseline/identity-queries.yaml",
		},
		"research": {
			dir:     "../../vaultmind-vault",
			queries: "../../test/fixtures/baseline/research-queries.yaml",
		},
	}
	spec, ok := vaults[name]
	if !ok {
		t.Fatalf("unknown vault %q — known: identity, research", name)
	}

	queries, err := baseline.LoadQueries(spec.queries)
	require.NoError(t, err)
	t.Logf("loaded %d golden queries for vault %q", len(queries), name)

	db, err := index.Open(filepath.Join(spec.dir, ".vaultmind", "index.db"))
	require.NoError(t, err)
	defer db.Close()

	xdg.SetAppName("vaultmind")
	expDBPath, err := xdg.DataFile("experiments.db")
	require.NoError(t, err)
	expDB, err := experiment.Open(expDBPath)
	require.NoError(t, err)
	defer func() { _ = expDB.Close() }()

	// 4-way: existing default. Cleanup BEFORE building the 5-way
	// retriever — hugot allows only one ORT session per process, and
	// holding two simultaneously falls back both to FTS-only and the
	// comparison becomes meaningless. Sequential init + run + cleanup
	// gives each retriever its full BGE-M3 lane access.
	res4 := query.BuildAutoRetrieverFull(db)
	rep4, err := baseline.Run(res4.Retriever, queries, baseline.RunConfig{K: 5, Limit: 10})
	res4.Cleanup()
	require.NoError(t, err)

	// 5-way: activation lane appended.
	res5 := query.BuildAutoRetrieverWithActivation(db, expDB)
	rep5, err := baseline.Run(res5.Retriever, queries, baseline.RunConfig{K: 5, Limit: 10})
	res5.Cleanup()
	require.NoError(t, err)

	// Aggregate report.
	t.Logf("")
	t.Logf("=== ACTIVATION LANE COMPARE — vault %q (n=%d) ===", name, len(queries))
	t.Logf("                   4-way        5-way        Δ")
	t.Logf("  Hit@5         %8.3f     %8.3f    %+.3f", rep4.HitAtK, rep5.HitAtK, rep5.HitAtK-rep4.HitAtK)
	t.Logf("  MRR           %8.3f     %8.3f    %+.3f", rep4.MRR, rep5.MRR, rep5.MRR-rep4.MRR)
	t.Logf("")

	// Per-query: list queries whose top-1 changed, and queries that lost rank.
	type queryDiff struct {
		query    string
		rr4, rr5 float64
		hit4     float64
		hit5     float64
	}
	byQuery4 := indexByQuery(rep4)
	byQuery5 := indexByQuery(rep5)
	diffs := make([]queryDiff, 0, len(queries))
	for q := range byQuery4 {
		r4 := byQuery4[q]
		r5 := byQuery5[q]
		diffs = append(diffs, queryDiff{
			query: q,
			rr4:   r4.ReciprocalRank,
			rr5:   r5.ReciprocalRank,
			hit4:  r4.HitAtK,
			hit5:  r5.HitAtK,
		})
	}
	sort.Slice(diffs, func(i, j int) bool {
		// Largest absolute MRR change first (positive or negative).
		di := diffs[i].rr5 - diffs[i].rr4
		dj := diffs[j].rr5 - diffs[j].rr4
		if di < 0 {
			di = -di
		}
		if dj < 0 {
			dj = -dj
		}
		return di > dj
	})

	t.Logf("Per-query rank-shift (sorted by |ΔRR| desc, top 20):")
	t.Logf("  %-50s rr4    rr5    Δ      hit4 hit5", "query")
	shown := 0
	for _, d := range diffs {
		delta := d.rr5 - d.rr4
		if delta == 0 && shown >= 5 {
			continue
		}
		marker := "  "
		switch {
		case delta > 0.001:
			marker = "▲ "
		case delta < -0.001:
			marker = "▼ "
		}
		t.Logf("  %s%-48s %.3f  %.3f  %+.3f  %.0f    %.0f",
			marker, truncate(d.query, 48), d.rr4, d.rr5, delta, d.hit4, d.hit5)
		shown++
		if shown >= 20 {
			break
		}
	}

	t.Logf("")
	t.Logf("Interpretation guide:")
	t.Logf("  ▲ = activation lane LIFTED the rank of this query's expected note")
	t.Logf("  ▼ = activation lane LOWERED the rank — possible drown-out by recently-accessed-but-irrelevant notes")
	t.Logf("  Δ Hit@5 < 0 anywhere = activation pushed an expected note out of top-5 entirely")
}

// TestActivationRerankSweep runs the curated golden-query set through the
// 4-way RRF (today's default) AND through the slice-5b” rerank variant
// across three (alpha, beta) weight configurations: {0.5/0.5, 0.7/0.3,
// 0.9/0.1}. Side-by-side report answers the α/β probe documented in
// reference-activation-rerank-decision: which weight pair wins on Hit@5
// + MRR for both vaults without degrading the 4-way baseline?
//
// Set VAULTMIND_RERANK_SWEEP=<vault> to run. Same vaults / fixtures as
// TestActivationLaneCompare.
func TestActivationRerankSweep(t *testing.T) {
	name := os.Getenv("VAULTMIND_RERANK_SWEEP")
	if name == "" {
		t.Skip("set VAULTMIND_RERANK_SWEEP=identity|research to run")
	}

	type vaultSpec struct {
		dir     string
		queries string
	}
	vaults := map[string]vaultSpec{
		"identity": {
			dir:     "../../vaultmind-identity",
			queries: "../../test/fixtures/baseline/identity-queries.yaml",
		},
		"research": {
			dir:     "../../vaultmind-vault",
			queries: "../../test/fixtures/baseline/research-queries.yaml",
		},
	}
	spec, ok := vaults[name]
	if !ok {
		t.Fatalf("unknown vault %q — known: identity, research", name)
	}

	queries, err := baseline.LoadQueries(spec.queries)
	require.NoError(t, err)
	t.Logf("loaded %d golden queries for vault %q", len(queries), name)

	db, err := index.Open(filepath.Join(spec.dir, ".vaultmind", "index.db"))
	require.NoError(t, err)
	defer db.Close()

	xdg.SetAppName("vaultmind")
	expDBPath, err := xdg.DataFile("experiments.db")
	require.NoError(t, err)
	expDB, err := experiment.Open(expDBPath)
	require.NoError(t, err)
	defer func() { _ = expDB.Close() }()

	// 4-way baseline first. Cleanup before each subsequent build —
	// hugot allows only one ORT session per process.
	res4 := query.BuildAutoRetrieverFull(db)
	rep4, err := baseline.Run(res4.Retriever, queries, baseline.RunConfig{K: 5, Limit: 10})
	res4.Cleanup()
	require.NoError(t, err)

	type variant struct {
		label string
		alpha float64
		beta  float64
	}
	variants := []variant{
		{"α0.5/β0.5", 0.5, 0.5},
		{"α0.7/β0.3", 0.7, 0.3},
		{"α0.9/β0.1", 0.9, 0.1},
	}
	type result struct {
		label string
		hit   float64
		mrr   float64
		dHit  float64
		dMRR  float64
	}
	results := make([]result, 0, len(variants))
	for _, v := range variants {
		rer := query.BuildAutoRetrieverWithRerank(db, expDB, v.alpha, v.beta)
		rep, runErr := baseline.Run(rer.Retriever, queries, baseline.RunConfig{K: 5, Limit: 10})
		rer.Cleanup()
		require.NoError(t, runErr)
		results = append(results, result{
			label: v.label,
			hit:   rep.HitAtK,
			mrr:   rep.MRR,
			dHit:  rep.HitAtK - rep4.HitAtK,
			dMRR:  rep.MRR - rep4.MRR,
		})
	}

	t.Logf("")
	t.Logf("=== ACTIVATION RERANK SWEEP — vault %q (n=%d) ===", name, len(queries))
	t.Logf("                         Hit@5    MRR     ΔHit@5   ΔMRR")
	t.Logf("  4-way (baseline)     %8.3f %8.3f       —        —", rep4.HitAtK, rep4.MRR)
	for _, r := range results {
		t.Logf("  rerank %-14s  %8.3f %8.3f  %+8.3f  %+8.3f", r.label, r.hit, r.mrr, r.dHit, r.dMRR)
	}
	t.Logf("")
	t.Logf("Decision criteria: pick the variant that maximizes MRR on this vault")
	t.Logf("WITHOUT degrading Hit@5 vs 4-way. If two tie within noise, prefer higher α (closer to no-rerank).")
}

// TestActivationRerankDeepDive runs ONE α/β configuration with full top-5
// dumps for all 19 identity / 40 research queries. Used to diagnose why
// the sweep shows Hit@5 collapses unexpectedly under modest β.
//
// VAULTMIND_RERANK_DEEPDIVE=identity:0.7:0.3 → vault=identity, α=0.7, β=0.3.
func TestActivationRerankDeepDive(t *testing.T) {
	spec := os.Getenv("VAULTMIND_RERANK_DEEPDIVE")
	if spec == "" {
		t.Skip("set VAULTMIND_RERANK_DEEPDIVE=<vault>:<alpha>:<beta>")
	}
	parts := splitCSVColon(spec)
	if len(parts) != 3 {
		t.Fatalf("VAULTMIND_RERANK_DEEPDIVE must be vault:alpha:beta")
	}
	vault := parts[0]
	alpha := parseFloat(t, parts[1])
	beta := parseFloat(t, parts[2])

	type vaultSpec struct {
		dir     string
		queries string
	}
	vaults := map[string]vaultSpec{
		"identity": {dir: "../../vaultmind-identity", queries: "../../test/fixtures/baseline/identity-queries.yaml"},
		"research": {dir: "../../vaultmind-vault", queries: "../../test/fixtures/baseline/research-queries.yaml"},
	}
	vs, ok := vaults[vault]
	if !ok {
		t.Fatalf("unknown vault %q", vault)
	}
	queries, err := baseline.LoadQueries(vs.queries)
	require.NoError(t, err)

	db, err := index.Open(filepath.Join(vs.dir, ".vaultmind", "index.db"))
	require.NoError(t, err)
	defer db.Close()
	xdg.SetAppName("vaultmind")
	expDBPath, err := xdg.DataFile("experiments.db")
	require.NoError(t, err)
	expDB, err := experiment.Open(expDBPath)
	require.NoError(t, err)
	defer func() { _ = expDB.Close() }()

	res4 := query.BuildAutoRetrieverFull(db)
	rep4, err := baseline.Run(res4.Retriever, queries, baseline.RunConfig{K: 5, Limit: 10})
	res4.Cleanup()
	require.NoError(t, err)

	rer := query.BuildAutoRetrieverWithRerank(db, expDB, alpha, beta)
	rep5, err := baseline.Run(rer.Retriever, queries, baseline.RunConfig{K: 5, Limit: 10})
	rer.Cleanup()
	require.NoError(t, err)

	t.Logf("")
	t.Logf("=== RERANK DEEP DIVE — vault=%q α=%.2f β=%.2f (n=%d) ===", vault, alpha, beta, len(queries))
	t.Logf("  4-way: Hit@5=%.3f MRR=%.3f", rep4.HitAtK, rep4.MRR)
	t.Logf("  rerank: Hit@5=%.3f MRR=%.3f", rep5.HitAtK, rep5.MRR)
	t.Logf("")

	by4 := indexByQuery(rep4)
	by5 := indexByQuery(rep5)
	t.Logf("Per-query (only when rank-1 OR Hit@5 changed):")
	for _, q := range rep4.Queries {
		r4 := by4[q.Text]
		r5 := by5[q.Text]
		if r4.ReciprocalRank == r5.ReciprocalRank && r4.HitAtK == r5.HitAtK {
			continue
		}
		t.Logf("Q: %q  rr4=%.3f rr5=%.3f  hit4=%.0f hit5=%.0f", q.Text, r4.ReciprocalRank, r5.ReciprocalRank, r4.HitAtK, r5.HitAtK)
		dump := func(label string, ids []string, expected []string) {
			expSet := make(map[string]bool)
			for _, e := range expected {
				expSet[e] = true
			}
			for i := 0; i < 5 && i < len(ids); i++ {
				m := "  "
				if expSet[ids[i]] {
					m = "✓ "
				}
				t.Logf("    %s %s rank=%d  %s", label, m, i+1, ids[i])
			}
		}
		dump("4-way:", r4.ResultIDs, r4.Expected)
		dump("rerank:", r5.ResultIDs, r5.Expected)
	}
}

// TestConfidenceCalibrationProbe captures the rank-1/rank-2 score gap
// distribution under both 4-way and 5b” rerank for the same query
// corpus. The 2026-04-30 calibration set TopHitConfidence thresholds
// 5% / 1.5% / 0.5% based on the 4-way distribution; this probe answers:
// does the rerank's blended-score distribution land on the same thresholds,
// or do they need re-tuning?
//
// Output: per-query gap% under each retriever, plus histogram of how
// queries classify under the current thresholds. If the histograms
// match between 4-way and rerank, current thresholds work. If many
// queries shift labels (strong → moderate, etc.), thresholds need
// re-calibration before 5b” can be wired as default.
//
// Set VAULTMIND_CONFIDENCE_CALIBRATION=<vault> to run.
func TestConfidenceCalibrationProbe(t *testing.T) {
	name := os.Getenv("VAULTMIND_CONFIDENCE_CALIBRATION")
	if name == "" {
		t.Skip("set VAULTMIND_CONFIDENCE_CALIBRATION=identity|research to run")
	}

	type vaultSpec struct {
		dir     string
		queries string
	}
	vaults := map[string]vaultSpec{
		"identity": {dir: "../../vaultmind-identity", queries: "../../test/fixtures/baseline/identity-queries.yaml"},
		"research": {dir: "../../vaultmind-vault", queries: "../../test/fixtures/baseline/research-queries.yaml"},
	}
	spec, ok := vaults[name]
	if !ok {
		t.Fatalf("unknown vault %q", name)
	}
	queries, err := baseline.LoadQueries(spec.queries)
	require.NoError(t, err)

	db, err := index.Open(filepath.Join(spec.dir, ".vaultmind", "index.db"))
	require.NoError(t, err)
	defer db.Close()
	xdg.SetAppName("vaultmind")
	expDBPath, err := xdg.DataFile("experiments.db")
	require.NoError(t, err)
	expDB, err := experiment.Open(expDBPath)
	require.NoError(t, err)
	defer func() { _ = expDB.Close() }()

	// Helper: run retriever on all queries, capture (top1, top2) scores
	// per query.
	type gapRow struct {
		query string
		top1  float64
		top2  float64
		gap   float64 // (top1 - top2) / top1
	}
	captureGaps := func(label string, ret retrieval.Retriever) []gapRow {
		rows := make([]gapRow, 0, len(queries))
		for _, q := range queries {
			results, _, runErr := ret.Search(context.Background(), q.Text, 10, 0, index.SearchFilters{})
			require.NoError(t, runErr)
			if len(results) < 2 || results[0].Score <= 0 {
				rows = append(rows, gapRow{query: q.Text, top1: 0, top2: 0, gap: 0})
				continue
			}
			gap := (results[0].Score - results[1].Score) / results[0].Score
			rows = append(rows, gapRow{query: q.Text, top1: results[0].Score, top2: results[1].Score, gap: gap})
		}
		return rows
	}
	classify := func(gap float64) string {
		switch {
		case gap >= 0.05:
			return "strong"
		case gap >= 0.015:
			return "moderate"
		case gap >= 0.005:
			return "weak"
		default:
			return "no_match"
		}
	}
	histogram := func(rows []gapRow) map[string]int {
		h := map[string]int{"strong": 0, "moderate": 0, "weak": 0, "no_match": 0}
		for _, r := range rows {
			h[classify(r.gap)]++
		}
		return h
	}

	// 4-way baseline.
	res4 := query.BuildAutoRetrieverFull(db)
	rows4 := captureGaps("4-way", res4.Retriever)
	res4.Cleanup()

	// 5b'' rerank with shipping defaults α=0.9/β=0.1.
	rer := query.BuildAutoRetrieverWithRerank(db, expDB, 0.9, 0.1)
	rows5 := captureGaps("rerank", rer.Retriever)
	rer.Cleanup()

	t.Logf("")
	t.Logf("=== CONFIDENCE CALIBRATION PROBE — vault %q (n=%d) ===", name, len(queries))
	h4 := histogram(rows4)
	h5 := histogram(rows5)
	t.Logf("                      strong  moderate  weak  no_match")
	t.Logf("  4-way (baseline)    %6d  %8d  %4d  %8d", h4["strong"], h4["moderate"], h4["weak"], h4["no_match"])
	t.Logf("  rerank α0.9/β0.1    %6d  %8d  %4d  %8d", h5["strong"], h5["moderate"], h5["weak"], h5["no_match"])
	t.Logf("")
	t.Logf("Per-query gap shifts (only when classification differs):")
	for i := range queries {
		c4 := classify(rows4[i].gap)
		c5 := classify(rows5[i].gap)
		if c4 != c5 {
			t.Logf("  %-50s  4-way %.3f%% (%s)  →  rerank %.3f%% (%s)",
				truncate(queries[i].Text, 50),
				rows4[i].gap*100, c4, rows5[i].gap*100, c5)
		}
	}
	t.Logf("")
	t.Logf("Decision: if rerank histogram matches 4-way within ±2 queries per bucket, current thresholds (5%%/1.5%%/0.5%%) hold. If buckets shift more, propose new thresholds and pin via TDD.")
}

// splitCSVColon splits "a:b:c" into ["a","b","c"].
func splitCSVColon(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func parseFloat(t *testing.T, s string) float64 {
	t.Helper()
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	require.NoError(t, err)
	return f
}

// indexByQuery flattens a Report's per-query results into a map keyed
// by query text, for side-by-side comparison.
func indexByQuery(r *baseline.Report) map[string]baseline.QueryResult {
	out := make(map[string]baseline.QueryResult, len(r.Queries))
	for _, q := range r.Queries {
		out[q.Text] = q
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// _ = context for keeping the import (in case future probe extensions
// want to thread a context through). Remove if unused.
var _ = context.Background

// _ = fmt for the same reason.
var _ = fmt.Sprintf
