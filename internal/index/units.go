package index

import (
	"database/sql"
	"fmt"

	"github.com/peiman/vaultmind/internal/section"
)

// Retrieval units. A note that fits the model's window is one unit, searched
// by its own vectors. A long note is searched by its sections (internal/
// section) once they have vectors of that kind: its own vector covers only
// the first 8,192 tokens, so it is left out then. Until its sections are
// embedded the note serves by its own vector, so it never drops out of search
// mid-upgrade. Only current sections count (note_hash = notes.hash); sections
// an older binary left stale are ignored. Short notes are untouched by all of
// this: a vault without long notes loads exactly the rows it did before.

// UnitID is the unit's own id: the section's for a section, else the note's.
func (ne NoteEmbedding) UnitID() string {
	if ne.SectionID != "" {
		return ne.SectionID
	}
	return ne.NoteID
}

// UnitID is the unit's own id: the section's for a section, else the note's.
func (ne NoteSparseEmbedding) UnitID() string {
	if ne.SectionID != "" {
		return ne.SectionID
	}
	return ne.NoteID
}

const (
	loadUnitDense = `SELECT n.id, '', n.embedding, n.type, n.title, n.path, n.body_text, n.is_domain
		FROM notes n
		WHERE n.embedding IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM sections s WHERE s.note_id = n.id AND s.note_hash = n.hash AND s.embedding IS NOT NULL)
		UNION ALL
		SELECT n.id, s.id, s.embedding, n.type, n.title, n.path, s.body, n.is_domain
		FROM sections s JOIN notes n ON n.id = s.note_id
		WHERE s.note_hash = n.hash AND s.embedding IS NOT NULL`
	loadUnitSparse = `SELECT n.id, '', n.sparse_embedding, n.type, n.title, n.path, n.body_text, n.is_domain
		FROM notes n
		WHERE n.sparse_embedding IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM sections s WHERE s.note_id = n.id AND s.note_hash = n.hash AND s.sparse_embedding IS NOT NULL)
		UNION ALL
		SELECT n.id, s.id, s.sparse_embedding, n.type, n.title, n.path, s.body, n.is_domain
		FROM sections s JOIN notes n ON n.id = s.note_id
		WHERE s.note_hash = n.hash AND s.sparse_embedding IS NOT NULL`
	loadNoteSectionVectors = `SELECT s.embedding FROM sections s JOIN notes n ON n.id = s.note_id
		WHERE s.note_id = ? AND s.note_hash = n.hash AND s.embedding IS NOT NULL ORDER BY s.ordinal`
)

// LoadUnitEmbeddings returns every retrieval unit with a dense vector: short
// notes, and the sections of long notes (each with its note's metadata and
// its own text). See the rules above.
func LoadUnitEmbeddings(d *DB) ([]NoteEmbedding, error) {
	rows, err := d.Query(loadUnitDense)
	if err != nil {
		return nil, fmt.Errorf("loading unit embeddings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []NoteEmbedding
	for rows.Next() {
		var ne NoteEmbedding
		var data []byte
		var noteType, title, path, body sql.NullString
		if err := rows.Scan(&ne.NoteID, &ne.SectionID, &data, &noteType, &title, &path, &body, &ne.IsDomain); err != nil {
			return nil, fmt.Errorf("scanning unit embedding: %w", err)
		}
		if ne.Embedding, err = DecodeEmbedding(data); err != nil {
			return nil, fmt.Errorf("decoding embedding for %q: %w", ne.UnitID(), err)
		}
		ne.Type, ne.Title, ne.Path, ne.BodyText = noteType.String, title.String, path.String, body.String
		out = append(out, ne)
	}
	return out, rows.Err()
}

// LoadUnitSparseEmbeddings is LoadUnitEmbeddings for the sparse lane.
func LoadUnitSparseEmbeddings(d *DB) ([]NoteSparseEmbedding, error) {
	rows, err := d.Query(loadUnitSparse)
	if err != nil {
		return nil, fmt.Errorf("loading unit sparse embeddings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []NoteSparseEmbedding
	for rows.Next() {
		var ne NoteSparseEmbedding
		var data []byte
		var noteType, title, path, body sql.NullString
		if err := rows.Scan(&ne.NoteID, &ne.SectionID, &data, &noteType, &title, &path, &body, &ne.IsDomain); err != nil {
			return nil, fmt.Errorf("scanning unit sparse embedding: %w", err)
		}
		if ne.Sparse, err = DecodeSparseEmbedding(data); err != nil {
			return nil, fmt.Errorf("decoding sparse for %q: %w", ne.UnitID(), err)
		}
		ne.Type, ne.Title, ne.Path, ne.BodyText = noteType.String, title.String, path.String, body.String
		out = append(out, ne)
	}
	return out, rows.Err()
}

// LoadNoteUnitEmbeddings returns the dense vectors that stand for one note in
// search: its current sections' once embedded, else its own (none when it has
// no vector). A relevance check scores a hit against the best of them.
func LoadNoteUnitEmbeddings(d *DB, noteID string) ([][]float32, error) {
	rows, err := d.Query(loadNoteSectionVectors, noteID)
	if err != nil {
		return nil, fmt.Errorf("loading section vectors for %q: %w", noteID, err)
	}
	var vecs [][]float32
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scanning section vector for %q: %w", noteID, err)
		}
		v, err := DecodeEmbedding(data)
		if err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("decoding section vector for %q: %w", noteID, err)
		}
		vecs = append(vecs, v)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(vecs) > 0 {
		return vecs, nil
	}
	v, err := LoadEmbedding(d, noteID)
	if err != nil || len(v) == 0 {
		return nil, err
	}
	return [][]float32{v}, nil
}

// SectionRow is one stored section of a long note.
type SectionRow struct {
	ID          string `json:"id"`
	NoteID      string `json:"note_id"`
	HeadingPath string `json:"heading_path"`
	Body        string `json:"body"`
}

const querySection = `SELECT s.id, s.note_id, s.heading_path, s.body
	FROM sections s JOIN notes n ON n.id = s.note_id
	WHERE s.id = ? AND s.note_hash = n.hash`

// QuerySection returns one current section by id — its text without its own
// heading line, which HeadingPath carries — or nil when there is no
// such section or it is stale (its note re-indexed by an older binary): a
// part of a note's old text is never served as current.
func QuerySection(d *DB, sectionID string) (*SectionRow, error) {
	var s SectionRow
	err := d.QueryRow(querySection, sectionID).Scan(&s.ID, &s.NoteID, &s.HeadingPath, &s.Body)
	if err == sql.ErrNoRows {
		return nil, nil //nolint:nilnil // not found is not an error: the caller falls back to the whole note
	}
	if err != nil {
		return nil, fmt.Errorf("querying section %q: %w", sectionID, err)
	}
	s.Body = section.WithoutHeading(s.Body, s.HeadingPath)
	return &s, nil
}
