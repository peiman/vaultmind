package experiment

import (
	"fmt"
	"math"
	"sort"
)

// JaccardAtK computes the Jaccard similarity of the top-K items in a and b.
// Order within each list does not affect the result — only set membership
// after truncation to K. Two empty lists return 1.0 (conventional). One empty
// and one non-empty returns 0.0.
func JaccardAtK(a, b []string, k int) float64 {
	aTop := truncate(a, k)
	bTop := truncate(b, k)
	if len(aTop) == 0 && len(bTop) == 0 {
		return 1.0
	}
	setA := toSet(aTop)
	setB := toSet(bTop)
	inter := 0
	for id := range setA {
		if _, ok := setB[id]; ok {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 1.0
	}
	return float64(inter) / float64(union)
}

func truncate(s []string, k int) []string {
	if k < 0 {
		k = 0
	}
	if len(s) <= k {
		return s
	}
	return s[:k]
}

func toSet(s []string) map[string]struct{} {
	out := make(map[string]struct{}, len(s))
	for _, v := range s {
		out[v] = struct{}{}
	}
	return out
}

// KendallTauShared computes Kendall's tau-a rank correlation restricted to
// items appearing in both a and b. Ranks are derived from list position
// (earlier = smaller rank). Returns (tau, sharedCount). If fewer than 2
// items are shared, returns (NaN, sharedCount) — rank correlation is
// undefined on <2 pairs and callers should treat NaN as "insufficient data."
func KendallTauShared(a, b []string) (float64, int) {
	rankA := make(map[string]int, len(a))
	for i, id := range a {
		rankA[id] = i
	}
	rankB := make(map[string]int, len(b))
	for i, id := range b {
		rankB[id] = i
	}
	shared := make([]string, 0, len(rankA))
	for id := range rankA {
		if _, ok := rankB[id]; ok {
			shared = append(shared, id)
		}
	}
	n := len(shared)
	if n < 2 {
		return math.NaN(), n
	}
	concordant, discordant := 0, 0
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			idI, idJ := shared[i], shared[j]
			da := rankA[idI] - rankA[idJ]
			db := rankB[idI] - rankB[idJ]
			prod := da * db
			switch {
			case prod > 0:
				concordant++
			case prod < 0:
				discordant++
			}
		}
	}
	totalPairs := n * (n - 1) / 2
	return float64(concordant-discordant) / float64(totalPairs), n
}

// EventPair is one (primary, shadow) comparison extracted from a single event's
// variants map. The two lists are note IDs in rank order (rank 1 first).
type EventPair struct {
	PrimaryVariant string
	ShadowVariant  string
	PrimaryList    []string
	ShadowList     []string
}

// ExtractEventPairs walks an event's decoded event_data JSON and returns one
// EventPair per shadow variant. Shadow order is deterministic (sorted by name).
// Returns an error if the primary variant is absent.
func ExtractEventPairs(eventData map[string]any, primaryName string) ([]EventPair, error) {
	variants, ok := eventData["variants"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("event_data has no variants map")
	}
	primaryList, err := extractRankList(variants, primaryName)
	if err != nil {
		return nil, fmt.Errorf("primary variant %q: %w", primaryName, err)
	}

	shadowNames := make([]string, 0, len(variants))
	for name := range variants {
		if name == primaryName {
			continue
		}
		shadowNames = append(shadowNames, name)
	}
	sort.Strings(shadowNames)

	pairs := make([]EventPair, 0, len(shadowNames))
	for _, name := range shadowNames {
		shadowList, err := extractRankList(variants, name)
		if err != nil {
			continue
		}
		pairs = append(pairs, EventPair{
			PrimaryVariant: primaryName,
			ShadowVariant:  name,
			PrimaryList:    primaryList,
			ShadowList:     shadowList,
		})
	}
	return pairs, nil
}

// ComparableEvent is one DB event after pair extraction. Events without
// shadow variants (or without a primary) should be filtered out before this
// slice is passed to AggregateComparisons.
type ComparableEvent struct {
	EventID string
	Pairs   []EventPair
}

// AggregateRow is the per-(primary, shadow) pair summary across many events.
// MeanKendallTau is a pointer because tau is undefined when no event in the
// pair had >=2 shared items; nil here means "insufficient data" and serializes
// to JSON null. Storing NaN here would break encoding/json (NaN is unsupported).
type AggregateRow struct {
	PrimaryVariant    string   `json:"primary_variant"`
	ShadowVariant     string   `json:"shadow_variant"`
	EventCount        int      `json:"event_count"`
	MeanJaccardAtK    float64  `json:"mean_jaccard_at_k"`
	MeanKendallTau    *float64 `json:"mean_kendall_tau"`
	KendallEventCount int      `json:"kendall_event_count"`
}

// AggregateComparisons collapses per-event EventPairs into per-pair
// aggregates. Jaccard is averaged over every event that contributed a pair.
// Kendall's tau averages only over events where the pair had >=2 shared
// items (others would contribute NaN). KendallEventCount reports how many
// events actually contributed to the tau average. Output rows are sorted by
// (primary, shadow) for deterministic display.
func AggregateComparisons(events []ComparableEvent, kCap int) []AggregateRow {
	type accum struct {
		events       int
		jaccardSum   float64
		kendallSum   float64
		kendallCount int
	}
	keyOf := func(a, b string) string { return a + "\x00" + b }
	byPair := map[string]*accum{}
	labels := map[string][2]string{}

	for _, ev := range events {
		for _, p := range ev.Pairs {
			pk := keyOf(p.PrimaryVariant, p.ShadowVariant)
			a, ok := byPair[pk]
			if !ok {
				a = &accum{}
				byPair[pk] = a
				labels[pk] = [2]string{p.PrimaryVariant, p.ShadowVariant}
			}
			a.events++
			a.jaccardSum += JaccardAtK(p.PrimaryList, p.ShadowList, effectiveK(kCap, len(p.PrimaryList), len(p.ShadowList)))
			tau, shared := KendallTauShared(p.PrimaryList, p.ShadowList)
			if shared >= 2 && !math.IsNaN(tau) {
				a.kendallSum += tau
				a.kendallCount++
			}
		}
	}

	keys := make([]string, 0, len(byPair))
	for k := range byPair {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([]AggregateRow, 0, len(keys))
	for _, mk := range keys {
		a := byPair[mk]
		lbl := labels[mk]
		row := AggregateRow{
			PrimaryVariant:    lbl[0],
			ShadowVariant:     lbl[1],
			EventCount:        a.events,
			MeanJaccardAtK:    a.jaccardSum / float64(a.events),
			KendallEventCount: a.kendallCount,
		}
		if a.kendallCount > 0 {
			mean := a.kendallSum / float64(a.kendallCount)
			row.MeanKendallTau = &mean
		}
		rows = append(rows, row)
	}
	return rows
}

// effectiveK returns kCap when positive, else the larger of the two list
// lengths. Guards against accidental K=0 from CLI defaults.
func effectiveK(kCap, lenA, lenB int) int {
	if kCap > 0 {
		return kCap
	}
	if lenA >= lenB {
		return lenA
	}
	return lenB
}

func extractRankList(variants map[string]any, name string) ([]string, error) {
	v, ok := variants[name].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("variant %q absent or malformed", name)
	}
	results, ok := v["results"].([]any)
	if !ok {
		return nil, fmt.Errorf("variant %q has no results array", name)
	}
	type rankedID struct {
		id   string
		rank int
	}
	ranked := make([]rankedID, 0, len(results))
	seen := make(map[string]struct{}, len(results))
	for _, r := range results {
		row, ok := r.(map[string]any)
		if !ok {
			continue
		}
		id, _ := row["note_id"].(string)
		if id == "" {
			continue
		}
		// Dedupe: keep the first occurrence (preserves the original rank
		// for that note_id). Without this, KendallTauShared's rank map gets
		// arbitrarily overwritten by later duplicates and produces wrong tau.
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		rank := 0
		switch rv := row["rank"].(type) {
		case float64:
			rank = int(rv)
		case int:
			rank = rv
		}
		ranked = append(ranked, rankedID{id: id, rank: rank})
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].rank < ranked[j].rank })
	out := make([]string, len(ranked))
	for i, r := range ranked {
		out[i] = r.id
	}
	return out, nil
}
