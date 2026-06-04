package distill

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Report is the propose-only result of a distillation scan: the candidate
// moments found, plus the corpus stats. It is NOT a set of arcs — every entry
// is a pointer for the mind to judge and (maybe) draft.
type Report struct {
	EpisodesScanned int
	EpisodesKept    int
	Candidates      []Candidate `json:"candidates"`
	// ParseErrors records episodes that failed to parse, surfaced rather than
	// silently skipped (distill is infrastructure and can't log, so the visible
	// error rides in the report itself).
	ParseErrors []string `json:"parse_errors,omitempty"`
}

// reportVerbatimMax caps how much of a (possibly long, multi-part) user turn the
// text report prints; the full text lives in the episode.
const reportVerbatimMax = 240

// FormatReport writes a human-readable, propose-only candidate report. It leads
// and closes with the contract — these are MOMENTS, not arcs; the mind drafts
// and approves — so the output can't be mistaken for finished identity.
func FormatReport(r Report, w io.Writer) error {
	if _, err := fmt.Fprintf(w,
		"Arc candidates — propose-only. These are MOMENTS, not arcs; you draft and approve (see how-to-write-arcs).\n\n"+
			"Scanned %d episodes (%d after signal filter) → %d candidate moments.\n",
		r.EpisodesScanned, r.EpisodesKept, len(r.Candidates)); err != nil {
		return err
	}
	if len(r.Candidates) == 0 {
		_, err := fmt.Fprintln(w, "\nNo candidate moments found.")
		return err
	}

	for _, ep := range groupByEpisode(r.Candidates) {
		if _, err := fmt.Fprintf(w, "\n## %s\n", ep.id); err != nil {
			return err
		}
		for _, c := range ep.candidates {
			if _, err := fmt.Fprintf(w, "  [%-15s via %q] turn %d: %q\n",
				c.Rule, c.Trigger, c.TurnIndex, truncate(oneLine(c.Verbatim), reportVerbatimMax)); err != nil {
				return err
			}
		}
	}

	for _, pe := range r.ParseErrors {
		if _, err := fmt.Fprintf(w, "\n! parse error (episode skipped): %s\n", pe); err != nil {
			return err
		}
	}

	_, err := fmt.Fprint(w,
		"\nA real arc needs a before/after shift in seeing, a verbatim push, and the cost — "+
			"many of these won't qualify. Draft the ones that did; ignore the rest. Never auto-write identity.\n")
	return err
}

type episodeGroup struct {
	id         string
	candidates []Candidate
}

// groupByEpisode buckets candidates by episode, episodes in id order, candidates
// in turn order — a stable, scannable layout.
func groupByEpisode(cands []Candidate) []episodeGroup {
	byEp := map[string][]Candidate{}
	for _, c := range cands {
		byEp[c.EpisodeID] = append(byEp[c.EpisodeID], c)
	}
	ids := make([]string, 0, len(byEp))
	for id := range byEp {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	groups := make([]episodeGroup, 0, len(ids))
	for _, id := range ids {
		cs := byEp[id]
		sort.SliceStable(cs, func(i, j int) bool { return cs[i].TurnIndex < cs[j].TurnIndex })
		groups = append(groups, episodeGroup{id: id, candidates: cs})
	}
	return groups
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
