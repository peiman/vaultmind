package cmd

import (
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/query"
)

// writePinnedSection prints how many notes are pinned and what they cost.
// A nil section was not measured and prints nothing. The listing is the id
// and the token cost: a title is free text, and the session-start hook greps
// doctor's lines. The warning names the remedy: unpin a note, or shorten one,
// until the set fits the session start.
func writePinnedSection(w io.Writer, p *query.DoctorPinned) error {
	if p == nil {
		return nil
	}
	if _, err := fmt.Fprintf(w, "Pinned notes: %d (%d tokens; session-start budget %d)\n", p.Count, p.Tokens, p.Budget); err != nil {
		return err
	}
	for _, n := range p.Notes {
		if _, err := fmt.Fprintf(w, "  %s  %d tokens\n", n.ID, n.Tokens); err != nil {
			return err
		}
	}
	if !p.OverBudget {
		return nil
	}
	_, err := fmt.Fprintf(w, "⚠ pinned notes exceed the session-start budget (%d tokens): unpin or shorten.\n", p.Budget)
	return err
}
