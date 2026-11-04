package index

import (
	"context"
	"fmt"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/passage"
)

// PassageVector stores a dense vector and its exact source span.
type PassageVector struct {
	UnitIdx, Start, End int
	Dense               []float32
}

// PassageSummary makes passage-only backfills visible in index's text summary.
func (r *EmbedResult) PassageSummary() string {
	if r.PassageEmbedded == 0 {
		return ""
	}
	return fmt.Sprintf("; passage vectors stored for %d notes", r.PassageEmbedded)
}

// LoadPassageVectors returns units in source order. Malformed blobs are errors,
// so the caller can fall back to lexical selection rather than use partial data.
func LoadPassageVectors(db *DB, noteID string) ([]PassageVector, error) {
	rows, err := db.Query(`SELECT unit_idx, start, end, dense FROM passage_vectors WHERE note_id=? ORDER BY unit_idx`, noteID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []PassageVector
	for rows.Next() {
		var v PassageVector
		var blob []byte
		if err := rows.Scan(&v.UnitIdx, &v.Start, &v.End, &blob); err != nil {
			return nil, err
		}
		v.Dense, err = DecodeEmbedding(blob)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type passageNote struct {
	id, title, body, raw string
	units                []passage.Unit
}

// passagePending also drives the lazy model-load decision. Filtering in Go
// uses the same comment stripper, token estimate, and spans as delivery.
func passagePending(db *DB) ([]passageNote, error) {
	rows, err := db.Query(`SELECT id, COALESCE(title,''), COALESCE(body_raw,body_text,'') FROM notes
	 WHERE embedding IS NOT NULL AND sparse_embedding IS NOT NULL AND colbert_embedding IS NOT NULL
	 AND NOT EXISTS (SELECT 1 FROM passage_vectors WHERE note_id=notes.id) ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var notes []passageNote
	for rows.Next() {
		var n passageNote
		if err := rows.Scan(&n.id, &n.title, &n.body); err != nil {
			return nil, err
		}
		n.raw = parser.StripHTMLComments(n.body)
		if passage.EstimateTokens(n.raw) <= passage.VectorThresholdTokens {
			continue
		}
		n.units = passage.Units(n.raw, n.title)
		if len(n.units) > 0 {
			notes = append(notes, n)
		}
	}
	return notes, rows.Err()
}

func supportsPassageVectors(e embedding.Embedder) bool {
	_, full := e.(embedding.FullEmbedder)
	_, sidecar := e.(*embedding.SidecarBGEM3Embedder)
	return full && !sidecar
}

func embedPassageNote(ctx context.Context, db *DB, e embedding.Embedder, n passageNote) error {
	texts, lengths := make([]string, len(n.units)), make([]int, len(n.units))
	for i, u := range n.units {
		texts[i] = noteEmbedText(n.title, n.raw[u.Start:u.End])
		lengths[i] = len(texts[i])
	}
	vectors := make([][]float32, len(texts))
	for _, batch := range planBGEM3Batches(lengths) {
		input := make([]string, len(batch))
		for i, j := range batch {
			input[i] = texts[j]
		}
		out, err := e.EmbedBatch(ctx, input)
		if err != nil {
			return fmt.Errorf("embedding passages for %q: %w", n.id, err)
		}
		if len(out) != len(batch) {
			return fmt.Errorf("passage vector count for %q: got %d, want %d", n.id, len(out), len(batch))
		}
		for i, j := range batch {
			if len(out[i]) != e.Dims() || len(out[i]) == 0 {
				return fmt.Errorf("invalid passage vector dimensions for %q", n.id)
			}
			vectors[j] = out[i]
		}
	}
	return storePassageNote(db, n, vectors)
}

// Store all units atomically. Recheck the source after the forward passes so
// an intervening note write cannot attach old spans to a new body.
func storePassageNote(db *DB, n passageNote, vectors [][]float32) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var current bool
	if err := tx.QueryRow(`SELECT COALESCE(body_raw,body_text,'')=? AND COALESCE(title,'')=? AND embedding IS NOT NULL FROM notes WHERE id=?`, n.body, n.title, n.id).Scan(&current); err != nil {
		return err
	}
	if !current {
		return fmt.Errorf("note %q changed during passage embedding; retry index --embed", n.id)
	}
	for i, u := range n.units {
		if _, err := tx.Exec(`INSERT INTO passage_vectors (note_id,unit_idx,start,end,dense) VALUES (?,?,?,?,?)`, n.id, i, u.Start, u.End, EncodeEmbedding(vectors[i])); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func finishPassageEmbedding(ctx context.Context, db *DB, e embedding.Embedder, result *EmbedResult, backfill int) (*EmbedResult, error) {
	if !supportsPassageVectors(e) {
		return result, nil
	}
	result.Skipped -= backfill
	notes, err := passagePending(db)
	if err != nil {
		return result, err
	}
	for _, n := range notes {
		if err := embedPassageNote(ctx, db, e, n); err != nil {
			return result, err
		}
		result.PassageEmbedded++
	}
	return result, nil
}
