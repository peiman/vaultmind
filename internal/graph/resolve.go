// Package graph provides entity resolution and graph traversal over the SQLite index.
package graph

import (
	"strings"

	"github.com/peiman/vaultmind/internal/index"
)

// Match holds one candidate note from a resolution attempt.
type Match struct {
	ID     string  `json:"id"`
	Type   string  `json:"type"`
	Title  string  `json:"title"`
	Path   string  `json:"path"`
	Status *string `json:"status,omitempty"`
}

// ResolveResult is the full output of entity resolution.
type ResolveResult struct {
	Resolved       bool    `json:"resolved"`
	Ambiguous      bool    `json:"ambiguous"`
	Input          string  `json:"input"`
	ResolutionTier *string `json:"resolution_tier"`
	Matches        []Match `json:"matches"`
}

// Resolver performs 5-tier entity resolution against the SQLite index.
type Resolver struct {
	db *index.DB
}

// NewResolver creates a Resolver backed by the given database.
func NewResolver(db *index.DB) *Resolver {
	return &Resolver{db: db}
}

// Resolve runs the full resolution cascade for the given input string.
// Priority: path shortcut → id → title → alias → normalized → unresolved.
func (r *Resolver) Resolve(input string) (*ResolveResult, error) {
	result := &ResolveResult{
		Input:   input,
		Matches: []Match{},
	}

	// Path shortcut: if input contains "/" or ends in ".md", try path first
	if strings.Contains(input, "/") || strings.HasSuffix(input, ".md") {
		row, err := r.db.QueryNoteByPath(input)
		if err != nil {
			return nil, err
		}
		if row != nil {
			return r.buildResult(result, []index.NoteRow{*row}, "path"), nil
		}
	}

	// Tier 1: exact id
	if row, err := r.db.QueryNoteByID(input); err != nil {
		return nil, err
	} else if row != nil {
		return r.buildResult(result, []index.NoteRow{*row}, "id"), nil
	}

	// Tier 2: exact title
	if rows, err := r.db.QueryNotesByTitle(input, false); err != nil {
		return nil, err
	} else if len(rows) > 0 {
		return r.buildResult(result, rows, "title"), nil
	}

	// Tier 3: exact alias
	if rows, err := r.db.QueryNotesByAlias(input, false); err != nil {
		return nil, err
	} else if len(rows) > 0 {
		return r.buildResult(result, rows, "alias"), nil
	}

	// Tier 4: normalized (case-insensitive title or alias)
	if rows, err := r.db.QueryNotesByTitle(input, true); err != nil {
		return nil, err
	} else if len(rows) > 0 {
		return r.buildResult(result, rows, "normalized"), nil
	}
	if rows, err := r.db.QueryNotesByAlias(input, true); err != nil {
		return nil, err
	} else if len(rows) > 0 {
		return r.buildResult(result, rows, "normalized"), nil
	}

	// Tier 5: unresolved
	return result, nil
}

func (r *Resolver) buildResult(result *ResolveResult, rows []index.NoteRow, tier string) *ResolveResult {
	result.Resolved = true
	result.ResolutionTier = &tier
	result.Ambiguous = len(rows) > 1

	for _, row := range rows {
		m := Match{
			ID:    row.ID,
			Type:  row.Type,
			Title: row.Title,
			Path:  row.Path,
		}
		if row.Status != "" {
			s := row.Status
			m.Status = &s
		}
		result.Matches = append(result.Matches, m)
	}
	return result
}
