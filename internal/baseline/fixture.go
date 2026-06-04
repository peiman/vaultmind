package baseline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultTolerance is the aggregate-metric drop allowed before a
// regression fires. 0.02 (2 percentage points) is tight enough to catch
// real degradations on a curated fixture but tolerant of small FTS
// scoring jitter between runs.
const DefaultTolerance = 0.02

// LoadQueries reads a golden-query fixture from YAML. The fixture is a
// flat list of Query entries (name/text/expected). Empty or missing
// files are errors — a baseline gate without queries measures nothing.
func LoadQueries(path string) ([]Query, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("reading query fixture %s: %w", path, err)
	}
	var queries []Query
	if err := yaml.Unmarshal(data, &queries); err != nil {
		return nil, fmt.Errorf("parsing query fixture %s: %w", path, err)
	}
	if len(queries) == 0 {
		return nil, fmt.Errorf("query fixture %s is empty — a baseline needs at least one query", path)
	}
	return queries, nil
}

// LoadSnapshot reads a committed baseline.json produced by a previous
// run. The JSON shape matches Report's tags, so snapshots round-trip
// through the runner's output without a mapping layer.
func LoadSnapshot(path string) (*Report, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("reading snapshot %s: %w", path, err)
	}
	var rep Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, fmt.Errorf("parsing snapshot %s: %w", path, err)
	}
	return &rep, nil
}

// Diff is the output of CompareToSnapshot. OK is the one-bit "passed
// the gate" answer; Regressions carries human-readable descriptions of
// every regressing dimension so operators don't have to re-derive what
// broke from a number.
type Diff struct {
	OK          bool
	Regressions []string
}

// CompareToSnapshot reports regressions between a current run and a
// committed snapshot. A regression is any aggregate or per-query metric
// that dropped below (snapshot - tolerance). Improvements never trigger
// the gate — callers can refresh the snapshot deliberately when a
// retrieval change is intended to raise quality.
//
// K mismatch is an operator error, not a silent comparison. Hit@5
// numbers compared against Hit@10 numbers would produce nonsense.
func CompareToSnapshot(current, snapshot *Report, tolerance float64) (*Diff, error) {
	if current.K != snapshot.K {
		return nil, fmt.Errorf("k mismatch: snapshot k=%d, current k=%d", snapshot.K, current.K)
	}

	diff := &Diff{OK: true}

	if current.HitAtK < snapshot.HitAtK-tolerance {
		diff.OK = false
		diff.Regressions = append(diff.Regressions,
			fmt.Sprintf("aggregate hit_at_k dropped: %.2f -> %.2f (tolerance %.2f)",
				snapshot.HitAtK, current.HitAtK, tolerance))
	}
	if current.MRR < snapshot.MRR-tolerance {
		diff.OK = false
		diff.Regressions = append(diff.Regressions,
			fmt.Sprintf("aggregate mrr dropped: %.2f -> %.2f (tolerance %.2f)",
				snapshot.MRR, current.MRR, tolerance))
	}

	// Build a name→snapshot-result map for per-query comparison so queries
	// that moved around in the file still compare correctly.
	byName := make(map[string]QueryResult, len(snapshot.Queries))
	for _, q := range snapshot.Queries {
		byName[q.Name] = q
	}
	for _, cur := range current.Queries {
		prev, ok := byName[cur.Name]
		if !ok {
			continue // new query not in snapshot — not a regression
		}
		if cur.HitAtK < prev.HitAtK-tolerance {
			diff.OK = false
			diff.Regressions = append(diff.Regressions,
				fmt.Sprintf("query %q hit_at_k dropped: %.2f -> %.2f (tolerance %.2f)",
					cur.Name, prev.HitAtK, cur.HitAtK, tolerance))
		}
		if cur.ReciprocalRank < prev.ReciprocalRank-tolerance {
			diff.OK = false
			diff.Regressions = append(diff.Regressions,
				fmt.Sprintf("query %q reciprocal_rank dropped: %.2f -> %.2f (tolerance %.2f)",
					cur.Name, prev.ReciprocalRank, cur.ReciprocalRank, tolerance))
		}
	}
	return diff, nil
}
