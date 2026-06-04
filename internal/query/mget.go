package query

import (
	"github.com/peiman/vaultmind/internal/index"
)

// MgetResult is the response for batch note reads.
type MgetResult struct {
	Notes    []index.FullNote `json:"notes"`
	NotFound []string         `json:"not_found"`
	Total    int              `json:"total"`
}

// Mget fetches multiple notes by ID. If frontmatterOnly is true,
// body/headings/blocks are omitted from each note.
func Mget(db *index.DB, ids []string, frontmatterOnly bool) (*MgetResult, error) {
	result := &MgetResult{
		Notes:    []index.FullNote{},
		NotFound: []string{},
	}

	for _, id := range ids {
		note, err := db.QueryFullNote(id)
		if err != nil {
			return nil, err
		}
		if note == nil {
			result.NotFound = append(result.NotFound, id)
			continue
		}
		if frontmatterOnly {
			note.Body = ""
			note.Headings = nil
			note.Blocks = nil
		}
		result.Notes = append(result.Notes, *note)
		result.Total++
	}

	return result, nil
}
