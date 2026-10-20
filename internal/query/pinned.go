package query

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
)

// DoctorPinnedNote is one pinned note and its full-body and delivered token costs.
type DoctorPinnedNote struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Tokens        int    `json:"tokens"`         // Full-body cost, before excerpting.
	SessionTokens int    `json:"session_tokens"` // Zero when omitted at session start.
}

// DoctorPinned is the pinned set measured against the default session-start budget.
// Notes and Omitted are empty, not null, when nothing is pinned.
type DoctorPinned struct {
	Count         int                `json:"count"`
	Tokens        int                `json:"tokens"` // Full-body total, before excerpting.
	SessionTokens int                `json:"session_tokens"`
	Budget        int                `json:"budget"`
	OverBudget    bool               `json:"over_budget"`
	Omitted       []string           `json:"omitted"`
	Notes         []DoctorPinnedNote `json:"notes"`
}

// attachPinned uses the hook's default excerpt and budget through memory.Pinned.
// The unbounded load preserves the full-body costs, including omitted notes.
// Hook-specific environment overrides are not available to doctor.
func attachPinned(db *index.DB, result *DoctorResult) error {
	full, err := memory.Pinned(db, memory.PinnedConfig{})
	if err != nil {
		return fmt.Errorf("pinned notes: %w", err)
	}
	loaded, err := memory.Pinned(db, memory.PinnedConfig{
		Budget: memory.SessionPinnedBudget, ExcerptTokens: memory.SessionPinnedExcerpt,
	})
	if err != nil {
		return fmt.Errorf("pinned notes at session start: %w", err)
	}

	sessionTokens := make(map[string]int, len(loaded.Items))
	for _, it := range loaded.Items {
		sessionTokens[it.ID] = it.Tokens
	}
	notes := make([]DoctorPinnedNote, 0, len(full.Items))
	for _, it := range full.Items {
		notes = append(notes, DoctorPinnedNote{
			ID: it.ID, Title: it.Title, Tokens: it.Tokens, SessionTokens: sessionTokens[it.ID],
		})
	}
	omitted := make([]string, 0, len(loaded.Omitted))
	for _, it := range loaded.Omitted {
		omitted = append(omitted, it.ID)
	}
	result.Pinned = &DoctorPinned{
		Count:         loaded.Total,
		Tokens:        full.Tokens,
		SessionTokens: loaded.Tokens,
		Budget:        memory.SessionPinnedBudget,
		OverBudget:    len(loaded.Omitted) > 0,
		Omitted:       omitted,
		Notes:         notes,
	}
	return nil
}
