package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/peiman/vaultmind/internal/query"
)

// writePinnedSection prints how many notes are pinned and what they cost.
// A nil section was not measured and prints nothing. The listing is the id
// and the token cost: a title is free text, and the session-start hook greps
// doctor's lines. The warning names notes the default session-start load omits.
func writePinnedSection(w io.Writer, p *query.DoctorPinned) error {
	if p == nil {
		return nil
	}
	line := fmt.Sprintf("Pinned notes: %d (%d tokens; session-start budget %d)", p.Count, p.Tokens, p.Budget)
	if p.Count > 0 {
		line = fmt.Sprintf("Pinned notes: %d (%d tokens at session start, %d in full; default budget %d)", p.Count, p.SessionTokens, p.Tokens, p.Budget)
	}
	if _, err := fmt.Fprintln(w, line); err != nil {
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
	_, err := fmt.Fprintf(w, "⚠ pinned notes that would not load at session start: %s — unpin or shorten.\n", strings.Join(p.Omitted, ", "))
	return err
}
