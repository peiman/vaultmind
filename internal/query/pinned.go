package query

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
)

// DoctorPinnedNote is one pinned note and the token cost of its whole body.
type DoctorPinnedNote struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Tokens int    `json:"tokens"`
}

// DoctorPinned is the pinned set measured against the session-start budget.
// Notes is empty, not null, when nothing is pinned.
type DoctorPinned struct {
	Count      int                `json:"count"`
	Tokens     int                `json:"tokens"`
	Budget     int                `json:"budget"`
	OverBudget bool               `json:"over_budget"`
	Notes      []DoctorPinnedNote `json:"notes"`
}

// attachPinned measures full bodies (budget 0, excerpt 0) so Tokens is the
// cost of the set a session would load before any excerpt. The fit rule is
// memory.Pinned, the same one the command uses.
func attachPinned(db *index.DB, result *DoctorResult) error {
	loaded, err := memory.Pinned(db, memory.PinnedConfig{})
	if err != nil {
		return fmt.Errorf("pinned notes: %w", err)
	}
	notes := make([]DoctorPinnedNote, 0, len(loaded.Items))
	for _, it := range loaded.Items {
		notes = append(notes, DoctorPinnedNote{ID: it.ID, Title: it.Title, Tokens: it.Tokens})
	}
	result.Pinned = &DoctorPinned{
		Count:      loaded.Total,
		Tokens:     loaded.Tokens,
		Budget:     memory.SessionPinnedBudget,
		OverBudget: loaded.Tokens > memory.SessionPinnedBudget,
		Notes:      notes,
	}
	return nil
}
