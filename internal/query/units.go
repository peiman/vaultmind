package query

import "github.com/peiman/vaultmind/internal/retrieval"

// firstPerNote keeps each note's best-scored hit. A lane scores units — a
// long note's sections each score on their own — but returns notes: one hit
// per note, naming the section that matched. results must be sorted best
// first. For a vault of short notes every unit is a note, so nothing changes.
func firstPerNote(results []retrieval.ScoredResult) []retrieval.ScoredResult {
	seen := make(map[string]bool, len(results))
	out := results[:0:0]
	for _, r := range results {
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		out = append(out, r)
	}
	return out
}

// page applies offset and limit to a lane's results, returning the page and
// the total before paging.
func page(results []retrieval.ScoredResult, limit, offset int) ([]retrieval.ScoredResult, int) {
	total := len(results)
	if offset >= total {
		return nil, total
	}
	results = results[offset:]
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, total
}
