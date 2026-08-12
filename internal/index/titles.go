package index

import "fmt"

// NoteTitle pairs a note's ID with its display title. Used by callers that
// need to list notes by title without loading full frontmatter/body (e.g.
// the ask command's fuzzy-title fallback on zero hits).
type NoteTitle struct {
	ID    string
	Title string
}

// NoteCount returns how many notes the index holds. Callers use it to decide
// whether corpus-scale judgements (e.g. noise-floor relevance) are meaningful
// for this vault at all, so it counts rows rather than loading them.
func (d *DB) NoteCount() (int, error) {
	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM notes").Scan(&n); err != nil {
		return 0, fmt.Errorf("counting notes: %w", err)
	}
	return n, nil
}

// AllNoteTitles returns every note's ID and title from the index. Titles
// that are empty in the notes table (shouldn't happen post-index but guard
// anyway) are returned as-is — callers filter if they care.
func (d *DB) AllNoteTitles() ([]NoteTitle, error) {
	rows, err := d.Query("SELECT id, title FROM notes")
	if err != nil {
		return nil, fmt.Errorf("querying note titles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []NoteTitle
	for rows.Next() {
		var nt NoteTitle
		if err := rows.Scan(&nt.ID, &nt.Title); err != nil {
			return nil, fmt.Errorf("scanning note title row: %w", err)
		}
		out = append(out, nt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating note title rows: %w", err)
	}
	return out, nil
}
