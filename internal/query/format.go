package query

import (
	"fmt"
	"io"
	"sort"
)

// FormatAsk writes a human-readable text representation of an AskResult.
func FormatAsk(result *AskResult, w io.Writer) error {
	return formatAskWithOptions(result, w, formatOpts{})
}

// FormatAskExplain is like FormatAsk but prints per-hit lane breakdowns
// (which sub-retrievers scored the note, each lane's raw 1/(K+rank), and
// how many lanes went into the mean). Lets you see the fusion math on
// the command line instead of piping --json through jq/python — closes
// the diagnostic gap that had operators rebuilding ad-hoc tooling for
// every ranking investigation.
func FormatAskExplain(result *AskResult, w io.Writer) error {
	return formatAskWithOptions(result, w, formatOpts{explain: true})
}

// FormatAskPointersOnly is like FormatAsk but skips body content for both
// the target note and every context-pack item — output is title + id +
// type only. Used by the SessionStart hook so the body of "what matters
// most right now" is never preloaded; the agent has to query for it.
//
// This converts the dogfood rule (use vaultmind ask before answering) from
// honor-system discipline (manifesto principle 9: discipline does not
// survive time pressure) to design: every body-read becomes an explicit,
// logged activation event the agent had to choose, rather than something
// the preload silently satisfied. Closes the trap documented in
// arc-plasticity-gap-from-inside and the 2026-04-25 design signal under
// step 3 of reference-plasticity-priority-order.
//
// Retrieval is unchanged — search hits, context-pack assembly, and
// scoring all happen normally. Only the rendering omits bodies. The
// hint at the bottom names the next move (an explicit ask) so the agent
// knows the loop closes by querying, not by waiting for more context.
func FormatAskPointersOnly(result *AskResult, w io.Writer) error {
	return formatAskWithOptions(result, w, formatOpts{pointersOnly: true})
}

// FormatAskPreview renders each ranked hit with a one-line snippet from
// the note body underneath the title — bridging the gap between
// pointers-only (titles, no body context) and full-body Ask (3000+
// tokens of context pack). Closes the AX gap named in the felt-
// experience inventory: with pointers-only I see ids and titles but
// often can't tell what a note actually is until I query its body.
// The snippet was already being populated by every retriever; this
// just renders it.
func FormatAskPreview(result *AskResult, w io.Writer) error {
	return formatAskWithOptions(result, w, formatOpts{preview: true})
}

type formatOpts struct {
	explain      bool
	pointersOnly bool
	preview      bool
}

func formatAskWithOptions(result *AskResult, w io.Writer, opts formatOpts) error {
	header := fmt.Sprintf("Search: %q (%d hits)", result.Query, len(result.TopHits))
	// Surface top-hit confidence inline when computable. This is the
	// signal the agent uses to distinguish "I'm sure about top-1" from
	// "top-1 might be coincidental, treat top-N as candidates." First
	// slice of plasticity-priority-order step 4 (calibrated confidence).
	// Hidden when confidence is empty (single-hit results, ill-defined
	// denominator) so the line stays terse for clear cases.
	if result.TopHitConfidence != "" {
		// no_match gets a longer label so the agent doesn't pattern-match
		// it as "weak" + a synonym. The whole point of the tier is to
		// distinguish "no clear winner" from "barely ahead but real."
		switch result.TopHitConfidence {
		case ConfidenceNoMatch:
			header += "  [top-hit confidence: no clear winner — top results essentially tied]"
		default:
			header += fmt.Sprintf("  [top-hit confidence: %s]", result.TopHitConfidence)
		}
	}
	if _, err := fmt.Fprintln(w, header); err != nil {
		return err
	}
	for _, h := range result.TopHits {
		if _, err := fmt.Fprintf(w, "  %.2f  %-40s  %s\n", h.Score, h.ID, h.Title); err != nil {
			return err
		}
		if opts.preview && h.Snippet != "" {
			if _, err := fmt.Fprintf(w, "        ↳ %s\n", Truncate(h.Snippet, 110)); err != nil {
				return err
			}
		}
		if opts.explain && len(h.Components) > 0 {
			if err := writeLaneBreakdown(w, h.Components); err != nil {
				return err
			}
		}
	}
	if result.Context == nil {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\nContext from: %s (%d items, %d/%d tokens)\n",
		result.Context.TargetID, len(result.Context.Context),
		result.Context.UsedTokens, result.Context.BudgetTokens); err != nil {
		return err
	}
	if result.Context.Target != nil {
		noteType, _ := result.Context.Target.Frontmatter["type"].(string)
		title, _ := result.Context.Target.Frontmatter["title"].(string)
		if _, err := fmt.Fprintf(w, "  [%s] %s\n", noteType, title); err != nil {
			return err
		}
		if !opts.pointersOnly && result.Context.Target.Body != "" {
			if _, err := fmt.Fprintf(w, "    %s\n", Truncate(result.Context.Target.Body, 120)); err != nil {
				return err
			}
		}
	}
	for _, item := range result.Context.Context {
		noteType, _ := item.Frontmatter["type"].(string)
		title, _ := item.Frontmatter["title"].(string)
		if _, err := fmt.Fprintf(w, "  [%s] %s\n", noteType, title); err != nil {
			return err
		}
		if !opts.pointersOnly && item.BodyIncluded && item.Body != "" {
			if _, err := fmt.Fprintf(w, "    %s\n", Truncate(item.Body, 120)); err != nil {
				return err
			}
		}
	}
	if opts.pointersOnly {
		// Hint the next move: pointers are not the answer, they're the menu.
		// Without this line the agent might treat the truncated output as
		// "all there is" instead of "here are the things to query for."
		if _, err := fmt.Fprintf(w, "\n(pointers only — run `vaultmind ask <query>` against any id above to read the body)\n"); err != nil {
			return err
		}
	}
	return nil
}

// Truncate shortens a string to maxLen runes, appending "..." if truncated.
func Truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// writeLaneBreakdown renders a single hit's per-lane RRF contributions in a
// deterministic form: lanes sorted alphabetically so diffs are reviewable,
// "mean of N" so readers can spot coverage imbalance at a glance (a hit
// with "mean of 2" next to one with "mean of 4" is the 2026-04-24 failure
// mode made visible without running SQL).
func writeLaneBreakdown(w io.Writer, components map[string]float64) error {
	lanes := make([]string, 0, len(components))
	for name := range components {
		lanes = append(lanes, name)
	}
	sort.Strings(lanes)

	if _, err := fmt.Fprint(w, "    lanes:"); err != nil {
		return err
	}
	for _, name := range lanes {
		if _, err := fmt.Fprintf(w, " %s=%.5f", name, components[name]); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "  mean of %d\n", len(lanes))
	return err
}
