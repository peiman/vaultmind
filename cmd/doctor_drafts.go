package cmd

import (
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/query"
)

// writeClosingReview prints folder-overview advice, then the draft and
// uncommitted lists. One call so writeDoctorHuman stays at its complexity.
func writeClosingReview(w io.Writer, result *query.DoctorResult) error {
	if err := writeOverviews(w, result.Overviews, result.VaultPath); err != nil {
		return err
	}
	return writeDraftReview(w, result)
}

// writeDraftReview prints the two review lists when the cmd layer measured
// them. A nil Notes slice means "not measured" (a partial result in tests)
// and prints nothing, so an empty measured list stays distinct.
func writeDraftReview(w io.Writer, result *query.DoctorResult) error {
	if result.Drafts.Notes != nil {
		if err := writeDrafts(w, result.Drafts); err != nil {
			return err
		}
	}
	if result.UncommittedNotes.Notes != nil || result.UncommittedNotes.Skipped {
		if err := writeUncommittedNotes(w, result.UncommittedNotes); err != nil {
			return err
		}
	}
	return writeGapReview(w, result.Gaps)
}

func writeDrafts(w io.Writer, d query.DoctorDrafts) error {
	if _, err := fmt.Fprintf(w, "Drafts awaiting review: %d\n", d.Count); err != nil {
		return err
	}
	for _, n := range d.Notes {
		if _, err := fmt.Fprintf(w, "  %s  %s  %dd\n", n.ID, n.Path, n.AgeDays); err != nil {
			return err
		}
	}
	return nil
}

func writeUncommittedNotes(w io.Writer, u query.DoctorUncommitted) error {
	if u.Skipped {
		_, err := fmt.Fprintf(w, "Uncommitted notes: skipped — %s\n", u.Reason)
		return err
	}
	if _, err := fmt.Fprintf(w, "Uncommitted notes older than %d days: %d\n", u.ThresholdDays, u.Count); err != nil {
		return err
	}
	for _, n := range u.Notes {
		if _, err := fmt.Fprintf(w, "  %s  %s  %dd\n", n.Path, n.State, n.AgeDays); err != nil {
			return err
		}
	}
	return nil
}
