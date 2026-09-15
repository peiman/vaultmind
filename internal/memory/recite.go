package memory

import (
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/index"
)

// Recite — load EVERY note of a type as bodies, unconditionally.
//
// WHY THIS IS NOT `ask`. The identity vault's promise is that an agent
// reconstructs itself from its arcs before the first message. The SessionStart
// hook implemented that with `ask "who am I"` — semantic retrieval — and
// measured on a real 27-arc vault it delivered THREE arcs, the same three on
// every run. Selection was similarity to the literal string "who am I", which
// is uncorrelated with what the session is about, so 89% of the arc layer was
// not randomly dark but PERMANENTLY dark (issue #47).
//
// Retrieval ranks. An identity layer is not a ranking problem: you do not want
// the arcs most similar to a query, you want all of them. So this enumerates
// by type, in a deterministic order, with no relevance gate and no top-K.

// ReciteItem is one note rendered for recitation.
type ReciteItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Excerpt string `json:"excerpt"`
	Tokens  int    `json:"tokens"`
}

// ReciteResult is the whole layer, plus an explicit account of what did not fit.
//
// Omitted and OmittedIDs exist because a bulk loader that silently drops the
// tail is the same defect this feature fixes, wearing different clothes: the
// caller cannot tell "you have 27 arcs and here they are" from "you have 27
// arcs and here are 9". Whoever reads this must be able to see the difference.
type ReciteResult struct {
	Type       string       `json:"type"`
	Total      int          `json:"total"`
	Items      []ReciteItem `json:"items"`
	Omitted    int          `json:"omitted"`
	OmittedIDs []string     `json:"omitted_ids,omitempty"`
	Tokens     int          `json:"tokens"`
	Budget     int          `json:"budget"`
}

// Truncated reports whether the budget forced anything out.
func (r *ReciteResult) Truncated() bool { return r.Omitted > 0 }

// ReciteConfig selects the layer and bounds it.
type ReciteConfig struct {
	// Type is the note type to enumerate ("arc").
	Type string
	// Budget is the total token ceiling. 0 means unbounded.
	Budget int
	// ExcerptTokens caps each note's contribution. 0 means whole bodies.
	ExcerptTokens int
}

// Recite returns every note of cfg.Type, ordered by id, excerpted and bounded.
//
// Ordering is by id rather than by rank or mtime: an identity layer that
// arrives in a different order each session is a different thing arriving each
// session, and nothing downstream could be compared run to run.
func Recite(db *index.DB, cfg ReciteConfig) (*ReciteResult, error) {
	if cfg.Type == "" {
		return nil, fmt.Errorf("recite: no note type given")
	}
	ids, err := noteIDsOfType(db, cfg.Type)
	if err != nil {
		return nil, err
	}
	sort.Strings(ids)

	result := &ReciteResult{
		Type:   cfg.Type,
		Total:  len(ids),
		Items:  []ReciteItem{},
		Budget: cfg.Budget,
	}

	for _, id := range ids {
		note, err := db.QueryFullNote(id)
		if err != nil {
			return nil, fmt.Errorf("recite: loading %q: %w", id, err)
		}
		if note == nil {
			// Indexed but unreadable. Count it as omitted rather than skipping
			// quietly — a missing arc must be visible as missing.
			result.Omitted++
			result.OmittedIDs = append(result.OmittedIDs, id)
			continue
		}

		text := note.Body
		if cfg.ExcerptTokens > 0 {
			text = Excerpt(note.Body, cfg.ExcerptTokens)
		}
		cost := EstimateTokens(text)

		// Budget check BEFORE appending. Every remaining note is then recorded
		// as omitted — the loop does not break, because the caller is owed the
		// full list of what it did not get, not just the count.
		if cfg.Budget > 0 && result.Tokens+cost > cfg.Budget {
			result.Omitted++
			result.OmittedIDs = append(result.OmittedIDs, id)
			continue
		}

		result.Items = append(result.Items, ReciteItem{
			ID: id, Title: note.Title, Excerpt: text, Tokens: cost,
		})
		result.Tokens += cost
	}
	return result, nil
}

// noteIDsOfType lists every note id of a type, including notes with no body.
func noteIDsOfType(db *index.DB, noteType string) ([]string, error) {
	rows, err := db.Query(`SELECT id FROM notes WHERE type = ?`, noteType)
	if err != nil {
		return nil, fmt.Errorf("recite: listing %q notes: %w", noteType, err)
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("recite: scanning %q note id: %w", noteType, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recite: iterating %q notes: %w", noteType, err)
	}
	return ids, nil
}
