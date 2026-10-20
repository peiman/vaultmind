package query

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/index"
)

// DoctorExampleNote is a starter note init wrote and tagged example.
type DoctorExampleNote struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// DoctorExampleNotes names those starters so they are not mistaken for the
// vault's own notes. Advice: delete them or keep them. Not an issue.
type DoctorExampleNotes struct {
	Count int                 `json:"count"`
	Notes []DoctorExampleNote `json:"notes"`
}

// exampleTag is the tag init puts on a sample note.
const exampleTag = "example"

// ListExampleNotes returns indexed notes tagged example, in path order.
func ListExampleNotes(db *index.DB) (DoctorExampleNotes, error) {
	rows, err := db.Query(`
		SELECT n.id, n.path
		FROM notes n
		JOIN tags t ON t.note_id = n.id
		WHERE t.tag = ?
		GROUP BY n.id, n.path
		ORDER BY n.path`, exampleTag)
	if err != nil {
		return DoctorExampleNotes{}, fmt.Errorf("listing example notes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := DoctorExampleNotes{Notes: []DoctorExampleNote{}}
	for rows.Next() {
		var n DoctorExampleNote
		if err := rows.Scan(&n.ID, &n.Path); err != nil {
			return DoctorExampleNotes{}, fmt.Errorf("listing example notes: %w", err)
		}
		out.Notes = append(out.Notes, n)
	}
	if err := rows.Err(); err != nil {
		return DoctorExampleNotes{}, fmt.Errorf("listing example notes: %w", err)
	}
	out.Count = len(out.Notes)
	return out, nil
}
