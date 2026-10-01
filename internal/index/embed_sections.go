package index

import (
	"context"
	"fmt"
	"strings"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/section"
	"github.com/rs/zerolog/log"
)

// SectionText is what a section is embedded from: the note's title and the
// section's heading path on one line, a blank line, then the section's text.
// A part read on its own loses what "it" refers to; the line says where it
// sits (title › heading › heading).
func SectionText(title, headingPath, body string) string {
	line := title
	if headingPath != "" {
		line += section.PathSeparator + headingPath
	}
	return line + "\n\n" + body
}

// The queries below select only sections cut from their note's current text
// (s.note_hash = n.hash): a section whose hash differs was left behind by a
// binary from before sections, which re-indexed the note without touching
// it. A section still to embed lacks dense, or with a full (BGE-M3) model
// sparse too. Sections have no ColBERT: on long documents it adds 0.2
// nDCG@10 over dense + sparse. Each query is one constant, written out.
const (
	countPendingSectionsFull = `SELECT COUNT(*) FROM sections s JOIN notes n ON n.id = s.note_id
		WHERE s.note_hash = n.hash AND (s.embedding IS NULL OR s.sparse_embedding IS NULL)`
	countPendingSectionsDense = `SELECT COUNT(*) FROM sections s JOIN notes n ON n.id = s.note_id
		WHERE s.note_hash = n.hash AND s.embedding IS NULL`
	countDoneSectionsFull = `SELECT COUNT(*) FROM sections s JOIN notes n ON n.id = s.note_id
		WHERE s.note_hash = n.hash AND s.embedding IS NOT NULL AND s.sparse_embedding IS NOT NULL`
	countDoneSectionsDense = `SELECT COUNT(*) FROM sections s JOIN notes n ON n.id = s.note_id
		WHERE s.note_hash = n.hash AND s.embedding IS NOT NULL`
	selectPendingSectionsFull = `SELECT s.id, COALESCE(NULLIF(n.title, ''), n.id), s.heading_path, s.body
		FROM sections s JOIN notes n ON n.id = s.note_id
		WHERE s.note_hash = n.hash AND (s.embedding IS NULL OR s.sparse_embedding IS NULL)
		ORDER BY s.note_id, s.ordinal`
	selectPendingSectionsDense = `SELECT s.id, COALESCE(NULLIF(n.title, ''), n.id), s.heading_path, s.body
		FROM sections s JOIN notes n ON n.id = s.note_id
		WHERE s.note_hash = n.hash AND s.embedding IS NULL
		ORDER BY s.note_id, s.ordinal`
)

// countSections returns the current sections still to embed and those done.
func countSections(db *DB, full bool) (pending, done int, err error) {
	pendingQ, doneQ := countPendingSectionsDense, countDoneSectionsDense
	if full {
		pendingQ, doneQ = countPendingSectionsFull, countDoneSectionsFull
	}
	if err := db.QueryRow(pendingQ).Scan(&pending); err != nil {
		return 0, 0, fmt.Errorf("counting pending sections: %w", err)
	}
	if err := db.QueryRow(doneQ).Scan(&done); err != nil {
		return 0, 0, fmt.Errorf("counting embedded sections: %w", err)
	}
	return pending, done, nil
}

type sectionText struct {
	id   string
	text string
}

// embedSections embeds the current sections of long notes that have no
// vectors yet: dense and sparse with a full model, dense with a dense-only
// one. It runs after the notes pass, on the same connection and embedder.
func embedSections(ctx context.Context, db *DB, embedder embedding.Embedder, result *EmbedResult) error {
	fullEmbedder, isFull := embedder.(embedding.FullEmbedder)
	query := selectPendingSectionsDense
	if isFull {
		query = selectPendingSectionsFull
	}
	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("querying unembedded sections: %w", err)
	}
	var pending []sectionText
	for rows.Next() {
		var id, title, path, body string
		if err := rows.Scan(&id, &title, &path, &body); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scanning unembedded section: %w", err)
		}
		if strings.TrimSpace(body) == "" {
			continue
		}
		pending = append(pending, sectionText{id: id, text: SectionText(title, path, body)})
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("closing unembedded section rows: %w", err)
	}

	batchSize := 32
	if isFull {
		batchSize = 8
	}
	lengths := make([]int, len(pending))
	for i, s := range pending {
		lengths[i] = len(s.text)
	}
	for _, idxs := range planEmbedBatches(lengths, batchSize, batchSize*embedPaddedBudgetPerSlot) {
		batch := make([]sectionText, len(idxs))
		texts := make([]string, len(idxs))
		for j, i := range idxs {
			batch[j], texts[j] = pending[i], pending[i].text
		}
		if isFull {
			embedSectionBatchFull(ctx, db, fullEmbedder, batch, texts, result)
		} else {
			embedSectionBatchDense(ctx, db, embedder, batch, texts, result)
		}
	}
	return nil
}

func embedSectionBatchFull(ctx context.Context, db *DB, emb embedding.FullEmbedder, batch []sectionText, texts []string, result *EmbedResult) {
	outs, err := emb.EmbedFullBatch(ctx, texts)
	if err != nil {
		log.Warn().Err(err).Int("batch_size", len(batch)).Msg("embedding section batch failed — sections in batch will lack embeddings")
		result.SectionsErrors += len(batch)
		return
	}
	writes := make([]sectionWrite, 0, len(batch))
	for j, out := range outs {
		// An empty sparse output is the heads producing nothing usable; leave
		// the section pending rather than store half of it (vaultmind#22).
		if len(out.Sparse) == 0 {
			log.Warn().Str("id", batch[j].id).Msg("BGE-M3 produced empty sparse output — section remains pending")
			result.SectionsEmptyOutput++
			continue
		}
		writes = append(writes, sectionWrite{id: batch[j].id, dense: EncodeEmbedding(out.Dense), sparse: EncodeSparseEmbedding(out.Sparse)})
	}
	storeSectionWrites(db, writes, result)
}

func embedSectionBatchDense(ctx context.Context, db *DB, emb embedding.Embedder, batch []sectionText, texts []string, result *EmbedResult) {
	vectors, err := emb.EmbedBatch(ctx, texts)
	if err != nil {
		log.Warn().Err(err).Int("batch_size", len(batch)).Msg("embedding section batch failed — sections in batch will lack embeddings")
		result.SectionsErrors += len(batch)
		return
	}
	writes := make([]sectionWrite, len(vectors))
	for j, v := range vectors {
		writes[j] = sectionWrite{id: batch[j].id, dense: EncodeEmbedding(v)}
	}
	storeSectionWrites(db, writes, result)
}

type sectionWrite struct {
	id     string
	dense  []byte
	sparse []byte // nil for a dense-only model
}

// storeSectionWrites stores one batch in one transaction: all of it, or none
// (counted as errors, left pending for the next pass).
func storeSectionWrites(db *DB, writes []sectionWrite, result *EmbedResult) {
	if len(writes) == 0 {
		return
	}
	tx, err := db.Begin()
	if err != nil {
		result.SectionsErrors += len(writes)
		return
	}
	for _, w := range writes {
		if _, err := tx.Exec(`UPDATE sections SET embedding = ?, sparse_embedding = ? WHERE id = ?`, w.dense, w.sparse, w.id); err != nil {
			log.Debug().Err(err).Str("id", w.id).Msg("storing section embedding failed")
			_ = tx.Rollback()
			result.SectionsErrors += len(writes)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		result.SectionsErrors += len(writes)
		return
	}
	result.SectionsEmbedded += len(writes)
}
