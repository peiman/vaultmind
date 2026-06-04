package memory

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/index"
)

// SummarizeConfig controls which notes to load and how much body text to include.
type SummarizeConfig struct {
	NoteIDs     []string
	IncludeBody bool
	MaxBodyLen  int // max chars per note body (0 = full)
}

// SummarizeSource holds assembled data for a single note.
type SummarizeSource struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Title       string                 `json:"title"`
	Frontmatter map[string]interface{} `json:"frontmatter"`
	BodyExcerpt string                 `json:"body_excerpt,omitempty"`
}

// SummarizeResult is the assembled output for agent consumption.
type SummarizeResult struct {
	Sources   []SummarizeSource `json:"sources"`
	NoteCount int               `json:"note_count"`
	NotFound  []string          `json:"not_found,omitempty"`
}

// Summarize loads frontmatter and optional body excerpts from the requested notes.
// It does not call an LLM — it assembles raw material for agent-driven synthesis.
func Summarize(db *index.DB, cfg SummarizeConfig) (*SummarizeResult, error) {
	result := &SummarizeResult{
		Sources: []SummarizeSource{},
	}

	for _, id := range cfg.NoteIDs {
		full, err := db.QueryFullNote(id)
		if err != nil {
			return nil, fmt.Errorf("querying note %q: %w", id, err)
		}
		if full == nil {
			result.NotFound = append(result.NotFound, id)
			continue
		}

		src := SummarizeSource{
			ID:          full.ID,
			Type:        full.Type,
			Title:       full.Title,
			Frontmatter: full.Frontmatter,
		}

		if cfg.IncludeBody {
			body := full.Body
			if cfg.MaxBodyLen > 0 && len(body) > cfg.MaxBodyLen {
				body = body[:cfg.MaxBodyLen] + "..."
			}
			src.BodyExcerpt = body
		}

		result.Sources = append(result.Sources, src)
	}

	result.NoteCount = len(result.Sources)
	return result, nil
}
