// Format helpers for `vaultmind experiment *` subcommands.
//
// Consolidated here so each experiment_<subcommand>.go can be wire-only
// (cobra command + run function + small result types) without each one
// shipping its own ad-hoc text formatter. Output shape evolves across
// summary/trace/compare together, not piecemeal.
package cmd

import (
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/experiment"
)

// ---- experiment summary ----

// formatUsageSummary renders a human-readable summary. Empty sections are
// silent rather than printing "0 notes" headers — a blank report beats a
// cluttered one.
func formatUsageSummary(s *experiment.UsageSummary, w io.Writer) error {
	if _, err := fmt.Fprintf(w, "Sessions: %d    Retrieval events: %d    Unique notes recalled: %d\n",
		s.TotalSessions, s.RetrievalEventCount, s.UniqueNotesRecalled); err != nil {
		return err
	}

	if s.GapStats.Count > 0 {
		if _, err := fmt.Fprintf(w, "\nSession gaps (%d): median %s, p90 %s, max %s\n",
			s.GapStats.Count,
			formatGap(s.GapStats.MedianSeconds),
			formatGap(s.GapStats.P90Seconds),
			formatGap(s.GapStats.MaxSeconds)); err != nil {
			return err
		}
	}

	if len(s.TopNotes) > 0 {
		if _, err := fmt.Fprintf(w, "\nTop recalled notes:\n"); err != nil {
			return err
		}
		for _, n := range s.TopNotes {
			if _, err := fmt.Fprintf(w, "  %4d  %s  (last %s)\n",
				n.RetrievalCountTotal, n.NoteID, n.LastRetrievedTs); err != nil {
				return err
			}
		}
	}

	return nil
}

// formatGap renders a seconds count as "Ns" / "Nm" / "Nh" / "Nd" depending
// on magnitude. Compact output for the terminal; precise seconds are in JSON.
func formatGap(seconds int64) string {
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%dm", seconds/60)
	case seconds < 86400:
		return fmt.Sprintf("%dh", seconds/3600)
	default:
		return fmt.Sprintf("%dd", seconds/86400)
	}
}

// ---- experiment trace ----

func formatSessionTrace(w io.Writer, t sessionTrace) error {
	who := callerLine(t.Caller, t.Meta)
	if _, err := fmt.Fprintf(w, "Session %s\n  %s\n  events: %d\n\n", t.SessionID, who, len(t.Events)); err != nil {
		return err
	}
	for _, e := range t.Events {
		query := e.Query
		if query == "" {
			query = "(no query text)"
		}
		if _, err := fmt.Fprintf(w, "  %s  %-12s  %q  → %d hits\n", e.Timestamp, e.EventType, query, len(e.Hits)); err != nil {
			return err
		}
		for _, h := range e.Hits {
			if _, err := fmt.Fprintf(w, "        rank %d  %s\n", h.Rank, h.NoteID); err != nil {
				return err
			}
		}
	}
	return nil
}

func formatNoteTrace(w io.Writer, t noteTrace) error {
	if _, err := fmt.Fprintf(w, "Note %s — %d retrievals\n\n", t.NoteID, len(t.Hits)); err != nil {
		return err
	}
	for _, h := range t.Hits {
		who := callerLine(h.Caller, h.Meta)
		if _, err := fmt.Fprintf(w, "  %s  rank %d  %-13s  session %s\n    %s\n",
			h.Timestamp, h.Rank, h.EventType, h.SessionID, who); err != nil {
			return err
		}
	}
	return nil
}

// callerLine renders the caller + operator as a single compact attribution
// string: "caller=workhorse-persona-hook  operator=peiman@MaxDaddy.local".
// Empty fields are omitted so unknown-caller rows don't show dangling "=".
func callerLine(caller string, meta map[string]any) string {
	parts := make([]string, 0, 3)
	if caller != "" {
		parts = append(parts, fmt.Sprintf("caller=%s", caller))
	} else {
		parts = append(parts, "caller=unknown")
	}
	user, _ := meta["user"].(string)
	host, _ := meta["host"].(string)
	if user != "" || host != "" {
		op := user
		if host != "" {
			if op != "" {
				op += "@" + host
			} else {
				op = host
			}
		}
		parts = append(parts, fmt.Sprintf("operator=%s", op))
	}
	if pd, _ := meta["claude_project_dir"].(string); pd != "" {
		parts = append(parts, fmt.Sprintf("project=%s", pd))
	}
	return joinSpaced(parts)
}

// joinSpaced joins strings with two spaces for compact terminal output.
func joinSpaced(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "  "
		}
		out += p
	}
	return out
}

// ---- experiment compare ----

// formatTau renders a *float64 tau as "%.3f" or "  nan" when undefined (nil).
func formatTau(t *float64) string {
	if t == nil {
		return "  nan"
	}
	return fmt.Sprintf("%.3f", *t)
}

func formatCompareResult(w io.Writer, r compareResult, k int, perEvent bool) error {
	if len(r.Aggregates) == 0 {
		_, err := fmt.Fprintln(w, "No comparable events found. Shadow variants may be disabled or no ask/search/context-pack events have been recorded.")
		return err
	}
	if _, err := fmt.Fprintf(w, "Variant disagreement (K=%d)\n\n", k); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  %-18s  %-18s  %6s  %11s  %11s  %9s\n",
		"primary", "shadow", "events", "meanJaccard", "meanKendall", "kendallN"); err != nil {
		return err
	}
	for _, a := range r.Aggregates {
		if _, err := fmt.Fprintf(w, "  %-18s  %-18s  %6d  %11.3f  %11s  %9d\n",
			a.PrimaryVariant, a.ShadowVariant, a.EventCount,
			a.MeanJaccardAtK, formatTau(a.MeanKendallTau), a.KendallEventCount); err != nil {
			return err
		}
	}
	if perEvent && len(r.PerEvent) > 0 {
		if _, err := fmt.Fprintln(w, "\nPer-event:"); err != nil {
			return err
		}
		for _, pe := range r.PerEvent {
			if _, err := fmt.Fprintf(w, "  %s  %s->%s  jaccard=%.3f  tau=%s  shared=%d\n",
				pe.EventID, pe.PrimaryVariant, pe.ShadowVariant,
				pe.JaccardAtK, formatTau(pe.KendallTau), pe.SharedItems); err != nil {
				return err
			}
		}
	}
	return nil
}
