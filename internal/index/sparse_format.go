package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/rs/zerolog/log"
)

const (
	metaKeySparseFormat = "sparse_format"

	// SparseFormatCurrent is the one stamp the embed path reads (Principle 7).
	// It is not the goose schema version, and it is never inferred from which
	// token ids a stored map contains. 2 means CLS, SEP, PAD, and UNK are
	// absent. Missing or lower means those tokens are still in the stored maps.
	SparseFormatCurrent = 2

	// SparseFormatStaleNotice and SparseFormatStaleRemedy are the words doctor,
	// ask, and a plain index share. One sentence, one command.
	SparseFormatStaleNotice = "sparse vectors are an older format; hybrid ranking still counts special tokens"
	SparseFormatStaleRemedy = "vaultmind index --embed"
)

// ReadSparseFormat returns the stamp. A missing row is 0, the old format.
func ReadSparseFormat(db *DB) (int, error) {
	var raw string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, metaKeySparseFormat).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("reading sparse format: %w", err)
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("sparse format %q: %w", raw, err)
	}
	return n, nil
}

// WriteSparseFormat stores the stamp. The caller writes it only after every
// stale sparse vector has been replaced.
func WriteSparseFormat(db *DB, format int) error {
	_, err := db.Exec(`INSERT INTO meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		metaKeySparseFormat, strconv.Itoa(format))
	if err != nil {
		return fmt.Errorf("writing sparse format: %w", err)
	}
	return nil
}

// SparseFormatStale reports that stored sparse vectors predate
// SparseFormatCurrent. A vault with no sparse vectors is not stale: there is
// nothing old to mix with a query. The decision is the stamp, not token id 0.
func SparseFormatStale(db *DB) (bool, error) {
	format, err := ReadSparseFormat(db)
	if err != nil {
		return false, err
	}
	if format >= SparseFormatCurrent {
		return false, nil
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notes WHERE sparse_embedding IS NOT NULL`).Scan(&n); err != nil {
		return false, fmt.Errorf("counting sparse embeddings: %w", err)
	}
	return n > 0, nil
}

// SparseFormatStaleFile opens dbPath and reports SparseFormatStale.
func SparseFormatStaleFile(dbPath string) (bool, error) {
	db, err := Open(dbPath) //nolint:contextcheck // Open does not accept a context; this is a stamp read
	if err != nil {
		return false, err
	}
	defer func() { _ = db.Close() }()
	return SparseFormatStale(db)
}

type sparseNote struct {
	id   string
	body string
}

// ReembedStaleSparse rewrites sparse_embedding for every note that has one
// when the stamp is older than SparseFormatCurrent. Dense and ColBERT bytes
// are not touched. The stamp is written only after every note succeeds. An
// empty new map is not stored and blocks the stamp. A fresh stamp returns
// without calling the embedder.
func (idx *Indexer) ReembedStaleSparse(ctx context.Context, db *DB, embedder embedding.Embedder, progress io.Writer) (int, error) {
	stale, err := SparseFormatStale(db)
	if err != nil {
		return 0, err
	}
	if !stale {
		return 0, nil
	}
	notes, err := loadSparseNotes(db)
	if err != nil {
		return 0, err
	}
	if len(notes) == 0 {
		return 0, nil
	}
	full, ok := embedder.(embedding.FullEmbedder)
	if !ok {
		return 0, fmt.Errorf("re-embedding sparse vectors: embedder cannot produce sparse vectors")
	}
	w := progress
	if w == nil && idx != nil {
		w = idx.Progress
	}
	if w == nil {
		w = os.Stderr
	}
	if _, err := fmt.Fprintf(w, "re-embedding sparse vectors for %d notes (format change)\n", len(notes)); err != nil {
		return 0, fmt.Errorf("reporting sparse re-embed: %w", err)
	}
	rewritten, passErr := reembedSparseNotes(ctx, db, full, notes)
	if passErr != nil { // sparse format stamp waits until this returns
		return rewritten, fmt.Errorf("re-embedding sparse vectors: %w", passErr)
	}
	if err := WriteSparseFormat(db, SparseFormatCurrent); err != nil {
		return rewritten, err
	}
	return rewritten, nil
}

func loadSparseNotes(db *DB) ([]sparseNote, error) {
	rows, err := db.Query(`SELECT id, COALESCE(body_text, '') FROM notes WHERE sparse_embedding IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("listing sparse notes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var notes []sparseNote
	for rows.Next() {
		var n sparseNote
		if err := rows.Scan(&n.id, &n.body); err != nil {
			return nil, fmt.Errorf("scanning sparse note: %w", err)
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing sparse notes: %w", err)
	}
	return notes, nil
}

func reembedSparseNotes(ctx context.Context, db *DB, full embedding.FullEmbedder, notes []sparseNote) (int, error) {
	lengths := make([]int, len(notes))
	for i, n := range notes {
		lengths[i] = len(n.body)
	}
	rewritten := 0
	for _, idxs := range planEmbedBatches(lengths, 8, 8*embedPaddedBudgetPerSlot) {
		texts := make([]string, len(idxs))
		for j, i := range idxs {
			texts[j] = notes[i].body
		}
		outs, err := full.EmbedFullBatch(ctx, texts)
		if err != nil {
			log.Debug().Err(err).Msg("sparse re-embed batch failed")
			return rewritten, err
		}
		n, err := storeSparseBatch(db, notes, idxs, outs)
		rewritten += n
		if err != nil {
			return rewritten, err
		}
	}
	return rewritten, nil
}

func storeSparseBatch(db *DB, notes []sparseNote, idxs []int, outs []*embedding.BGEM3Output) (int, error) {
	if len(outs) != len(idxs) {
		return 0, fmt.Errorf("sparse re-embed: got %d outputs for %d notes", len(outs), len(idxs))
	}
	stored := 0
	for j, i := range idxs {
		out := outs[j]
		if out == nil || len(out.Sparse) == 0 {
			log.Debug().Str("id", notes[i].id).Msg("sparse re-embed produced an empty map")
			return stored, fmt.Errorf("empty sparse embedding for %q", notes[i].id)
		}
		if err := StoreSparseEmbedding(db, notes[i].id, out.Sparse); err != nil {
			log.Debug().Err(err).Str("id", notes[i].id).Msg("storing re-embedded sparse vector failed")
			return stored, err
		}
		stored++
	}
	return stored, nil
}

// sparseFormatBlocksLazy is true when an incremental embed must load the
// model even though no note is missing vectors: the sparse column is stale.
func sparseFormatBlocksLazy(dbPath, model string) (bool, error) {
	if model != embedding.ModelBGEM3 {
		return false, nil
	}
	return SparseFormatStaleFile(dbPath)
}

// skipIncrementalEmbed reports whether RunEmbed should return before loading
// the model. skip is false when notes are pending or the sparse column is stale.
func skipIncrementalEmbed(dbPath, model string) (skip bool, res EmbedResult, err error) {
	if err := guardEmbeddingModel(dbPath, model); err != nil {
		return false, EmbedResult{}, err
	}
	//nolint:contextcheck // countPendingForModel calls Open which does not accept a context
	pending, skipped, err := countPendingForModel(dbPath, model)
	if err != nil {
		return false, EmbedResult{}, err
	}
	if pending > 0 {
		return false, EmbedResult{}, nil
	}
	block, err := sparseFormatBlocksLazy(dbPath, model)
	if err != nil {
		return false, EmbedResult{}, err
	}
	if block {
		return false, EmbedResult{}, nil
	}
	return true, EmbedResult{Skipped: skipped, Model: model}, nil
}
