package distill

import (
	"context"
	"fmt"
	"strings"
)

// NearArc is an existing arc that resembles a proposed one, with the similarity
// that says how much. It is evidence for a human judgement, never the judgement.
type NearArc struct {
	ID    string  `json:"id"`
	Title string  `json:"title,omitempty"`
	Score float64 `json:"score"`
}

// ArcFinder returns the existing arcs most similar to text, best first. It is an
// interface so this package stays free of the index and retrieval layers, and so
// the annotation logic can be tested without a vault.
type ArcFinder interface {
	NearestArcs(ctx context.Context, text string, limit int) ([]NearArc, error)
}

// AnnotateNearestArcs attaches similar existing arcs to every proposal.
//
// This is the mitigation for what the 2026-05-31 distillation review called the
// single biggest risk in the pipeline: "de-duplication, not extraction". That
// review measured the failure directly — two independent miners mis-tagged the
// same candidate to two different existing arcs, both wrong. Extraction was
// reliable; attribution was not.
//
// So the division of labour is deliberate. Finding the neighbours is mechanical
// and reliable, and is automated here. Deciding whether a proposal is ALREADY
// COVERED is neither, and is left to the reader — the report shows scores and
// never a verdict. The asymmetry of the two mistakes justifies it: a wrongly
// proposed duplicate costs a few seconds of reading, while a wrong "already
// covered" silently discards a transformation nobody will look for again.
//
// A nil finder (unindexed vault) is a clean no-op. A finder that fails degrades
// to no neighbours and records why — the proposals themselves always survive,
// because losing them to protect an aid would invert their importance.
func AnnotateNearestArcs(ctx context.Context, r Report, f ArcFinder, limit int) Report {
	if f == nil {
		return r
	}
	// One line per distinct failure REASON, counted.
	//
	// The first version keyed on the formatted message, which embedded the
	// per-proposal subject — so two proposals never produced an equal string and
	// nothing ever collapsed. A locked index across 27 proposals emitted 27
	// identical-in-substance lines, exactly the outcome the code claimed to
	// prevent. Keying on the error text is what the comment always meant.
	counts := map[string]int{}
	order := []string{}
	note := func(_ string, err error) {
		reason := err.Error()
		if _, seen := counts[reason]; !seen {
			order = append(order, reason)
		}
		counts[reason]++
	}

	for i := range r.DeskPending {
		text := matchText(r.DeskPending[i].Snippet, r.DeskPending[i].Title)
		near, err := lookup(ctx, f, text, limit)
		if err != nil {
			note(r.DeskPending[i].ID, err)
			continue
		}
		r.DeskPending[i].NearestArcs = near
	}
	for i := range r.Candidates {
		near, err := lookup(ctx, f, matchText(r.Candidates[i].Verbatim), limit)
		if err != nil {
			note(r.Candidates[i].EpisodeID, err)
			continue
		}
		r.Candidates[i].NearestArcs = near
	}

	for _, reason := range order {
		r.Diagnostics = append(r.Diagnostics,
			fmt.Sprintf("nearest-arc lookup failed for %d proposal(s): %s", counts[reason], reason))
	}
	return r
}

// lookup skips the query when there is nothing to match on, so an entry missing
// both body and title costs nothing rather than returning the vault's generic
// nearest neighbours to an empty string.
func lookup(ctx context.Context, f ArcFinder, text string, limit int) ([]NearArc, error) {
	if text == "" {
		return nil, nil
	}
	return f.NearestArcs(ctx, text, limit)
}

// matchText picks the first non-empty candidate. A desk entry's body carries the
// transformation while its title is only a headline, so the body is preferred —
// matching on titles alone finds keyword coincidences rather than kin.
func matchText(candidates ...string) string {
	for _, c := range candidates {
		if s := strings.TrimSpace(c); s != "" {
			return s
		}
	}
	return ""
}
