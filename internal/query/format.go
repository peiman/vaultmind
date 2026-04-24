package query

import (
	"fmt"
	"io"
	"sort"
)

// FormatAsk writes a human-readable text representation of an AskResult.
func FormatAsk(result *AskResult, w io.Writer) error {
	return formatAskWithOptions(result, w, false)
}

// FormatAskExplain is like FormatAsk but prints per-hit lane breakdowns
// (which sub-retrievers scored the note, each lane's raw 1/(K+rank), and
// how many lanes went into the mean). Lets you see the fusion math on
// the command line instead of piping --json through jq/python — closes
// the diagnostic gap that had operators rebuilding ad-hoc tooling for
// every ranking investigation.
func FormatAskExplain(result *AskResult, w io.Writer) error {
	return formatAskWithOptions(result, w, true)
}

func formatAskWithOptions(result *AskResult, w io.Writer, explain bool) error {
	if _, err := fmt.Fprintf(w, "Search: %q (%d hits)\n", result.Query, len(result.TopHits)); err != nil {
		return err
	}
	for _, h := range result.TopHits {
		if _, err := fmt.Fprintf(w, "  %.2f  %-40s  %s\n", h.Score, h.ID, h.Title); err != nil {
			return err
		}
		if explain && len(h.Components) > 0 {
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
		if result.Context.Target.Body != "" {
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
		if item.BodyIncluded && item.Body != "" {
			if _, err := fmt.Fprintf(w, "    %s\n", Truncate(item.Body, 120)); err != nil {
				return err
			}
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
