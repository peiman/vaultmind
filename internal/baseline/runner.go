package baseline

import (
	"context"
	"fmt"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
)

// Query is a single golden-query spec. Name is a short label for
// per-query reporting; Text is the actual search string; Expected is the
// curated set of note IDs a well-behaved retriever should surface in the
// top-K (order among Expected is not significant — see ReciprocalRank).
type Query struct {
	Name     string   `yaml:"name"     json:"name"`
	Text     string   `yaml:"text"     json:"text"`
	Expected []string `yaml:"expected" json:"expected"`
}

// RunConfig controls per-run behavior.
//
//	K     — rank cutoff for Hit@K (typically 5 or 10).
//	Limit — max results requested from the retriever. Should be ≥ K;
//	        lower limits make Hit@K pessimistic because the retriever
//	        never gets a chance to place expected IDs beyond the cap.
type RunConfig struct {
	K     int
	Limit int
}

// QueryResult is one row of a Report: the resolved top-K IDs plus
// per-query metrics. ResultIDs is deliberately included so a regression
// is *diagnosable* — a number without provenance can't be debugged.
type QueryResult struct {
	Name           string   `json:"name"`
	Text           string   `json:"text"`
	Expected       []string `json:"expected"`
	ResultIDs      []string `json:"result_ids"`
	HitAtK         float64  `json:"hit_at_k"`
	ReciprocalRank float64  `json:"reciprocal_rank"`
}

// Report is the full baseline run — one QueryResult per input query plus
// aggregate Hit@K and MRR (means across queries).
type Report struct {
	K       int           `json:"k"`
	Queries []QueryResult `json:"queries"`
	HitAtK  float64       `json:"hit_at_k"`
	MRR     float64       `json:"mrr"`
}

// Run executes each query through the retriever and builds a Report. Miss
// queries (no expected ID in top-K) register as 0.0 rows rather than
// errors — partial failure is a signal, not a crash.
//
// A retriever failure on any single query aborts the whole run: silently
// dropping a query would hide the failure behind an artificially lower
// aggregate, which is the class of bug baselines exist to catch.
func Run(retriever query.Retriever, queries []Query, cfg RunConfig) (*Report, error) {
	rep := &Report{K: cfg.K, Queries: make([]QueryResult, 0, len(queries))}
	if len(queries) == 0 {
		return rep, nil
	}

	var hitSum, rrSum float64
	for _, q := range queries {
		results, _, err := retriever.Search(context.Background(), q.Text, cfg.Limit, 0, index.SearchFilters{})
		if err != nil {
			return nil, fmt.Errorf("query %q (%q): %w", q.Name, q.Text, err)
		}
		ids := make([]string, len(results))
		for i, r := range results {
			ids[i] = r.ID
		}
		hit := HitAtK(ids, q.Expected, cfg.K)
		rr := ReciprocalRank(ids, q.Expected)
		rep.Queries = append(rep.Queries, QueryResult{
			Name: q.Name, Text: q.Text, Expected: q.Expected,
			ResultIDs: ids, HitAtK: hit, ReciprocalRank: rr,
		})
		hitSum += hit
		rrSum += rr
	}

	n := float64(len(queries))
	rep.HitAtK = hitSum / n
	rep.MRR = rrSum / n
	return rep, nil
}
