package index

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/peiman/vaultmind/internal/embedding"
)

const (
	metaKeyEmbedTextFormat = "embed_text_format"

	// EmbedTextFormatCurrent is the stamp the embed path reads (Principle 7).
	// 2 means the dense vector was computed from the title and the body.
	// Missing or lower means the vector was computed from the body only.
	EmbedTextFormatCurrent = 2

	// EmbedTextStaleNotice and EmbedTextStaleRemedy are the words doctor,
	// ask, and a plain index share. One sentence, one command.
	EmbedTextStaleNotice = "embeddings predate title embedding; note titles do not count in semantic ranking"
	EmbedTextStaleRemedy = "vaultmind index --embed"
)

// ReadEmbedTextFormat returns the stamp. A missing row is 0, body only.
func ReadEmbedTextFormat(db *DB) (int, error) {
	var raw string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, metaKeyEmbedTextFormat).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("reading embed text format: %w", err)
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("embed text format %q: %w", raw, err)
	}
	return n, nil
}

// WriteEmbedTextFormat stores the stamp. A caller stamps a vault that was
// not stale before EmbedNotes runs, and stamps an upgrade after that pass
// finishes. A failure to write the row does not leave a partial value.
func WriteEmbedTextFormat(db *DB, format int) error {
	_, err := db.Exec(`INSERT INTO meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		metaKeyEmbedTextFormat, strconv.Itoa(format))
	if err != nil {
		return fmt.Errorf("writing embed text format: %w", err)
	}
	return nil
}

// EmbedTextStale reports that stored dense vectors predate title embedding.
// A vault with no dense vectors is not stale: there is nothing old to mix
// with a query. The decision is the stamp, not the model.
func EmbedTextStale(db *DB) (bool, error) {
	format, err := ReadEmbedTextFormat(db)
	if err != nil {
		return false, err
	}
	if format >= EmbedTextFormatCurrent {
		return false, nil
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notes WHERE embedding IS NOT NULL`).Scan(&n); err != nil {
		return false, fmt.Errorf("counting dense embeddings: %w", err)
	}
	return n > 0, nil
}

// EmbedTextStaleFile opens dbPath and reports EmbedTextStale.
func EmbedTextStaleFile(dbPath string) (bool, error) {
	db, err := Open(dbPath) //nolint:contextcheck // Open does not accept a context; this is a stamp read
	if err != nil {
		return false, err
	}
	defer func() { _ = db.Close() }()
	return EmbedTextStale(db)
}

// embedTextPlan says what this pass does with the title stamp.
// upgrade re-embeds every note and skips the sparse-only migration.
// writeText is a vault that was not stale on the way in. The text stamp is
// written before EmbedNotes, so a failed batch cannot leave title vectors
// under stamp 0.
type embedTextPlan struct {
	upgrade   bool
	writeText bool
}

func embedTextPlanFor(dbPath string, upgrade bool) (embedTextPlan, error) {
	stale, err := EmbedTextStaleFile(dbPath)
	if err != nil {
		return embedTextPlan{}, err
	}
	if upgrade && stale {
		return embedTextPlan{upgrade: true}, nil
	}
	if !stale {
		return embedTextPlan{writeText: true}, nil
	}
	return embedTextPlan{}, nil
}

// prepareEmbedText reads the stamp before a purge can clear the dense rows
// that make a vault look stale, and arms EmbedNotes when this pass rewrites
// every note.
func (idx *Indexer) prepareEmbedText(dbPath string) (embedTextPlan, error) {
	plan, err := embedTextPlanFor(dbPath, idx.UpgradeEmbedText)
	if err != nil {
		return embedTextPlan{}, err
	}
	idx.reembedAll = plan.upgrade
	return plan, nil
}

// stampTextIfCurrent writes the text stamp before EmbedNotes when the vault
// was not stale. Notes embedded after this are title vectors. Notes a batch
// does not store have no vectors. The stamp matches either way.
func stampTextIfCurrent(plan embedTextPlan, dbPath string) error {
	if !plan.writeText {
		return nil
	}
	return writeEmbedTextStamp(dbPath)
}

func writeEmbedTextStamp(dbPath string) error {
	db, err := Open(dbPath) //nolint:contextcheck // Open does not accept a context; this is a stamp write
	if err != nil {
		return fmt.Errorf("opening index database to stamp embed text format: %w", err)
	}
	defer func() { _ = db.Close() }()
	return WriteEmbedTextFormat(db, EmbedTextFormatCurrent)
}

// writeUpgradeStamps records both stamps after a title upgrade. Sparse is
// written first: a failure there leaves the text stamp unset, so the next
// index --embed tries the whole pass again.
func writeUpgradeStamps(dbPath, model string) error {
	db, err := Open(dbPath) //nolint:contextcheck // Open does not accept a context; this is a stamp write
	if err != nil {
		return fmt.Errorf("opening index database to stamp embed text format: %w", err)
	}
	defer func() { _ = db.Close() }()
	if model == embedding.ModelBGEM3 {
		if err := WriteSparseFormat(db, SparseFormatCurrent); err != nil {
			return err
		}
	}
	return WriteEmbedTextFormat(db, EmbedTextFormatCurrent)
}

// clearNoteVectors drops dense, sparse, and colbert together. Migration 006
// aborts an update that leaves a 1024-d dense vector without both companions,
// so a note a title upgrade did not rewrite is cleared in one statement.
// The note then looks like one that was never embedded.
func clearNoteVectors(db *DB, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("clearing note vectors: %w", err)
	}
	for _, id := range ids {
		if _, err := tx.Exec(
			`UPDATE notes SET embedding = NULL, sparse_embedding = NULL, colbert_embedding = NULL WHERE id = ?`,
			id); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("clearing note vectors: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("clearing note vectors: %w", err)
	}
	return nil
}

// stampEmbedPass writes the stamps a finished pass earned.
// A title upgrade writes both stamps when the pass returned and the context
// was not cancelled, including when some notes were cleared instead of
// rewritten. Any other pass leaves the text stamp to the write that happened
// before EmbedNotes, and writes the sparse stamp only when the pass stored
// vectors and reported no errors.
func stampEmbedPass(dbPath, model string, upgraded int, plan embedTextPlan, res *EmbedResult, cancelled bool) error {
	if res == nil {
		return nil
	}
	if plan.upgrade {
		if cancelled {
			return nil
		}
		return writeUpgradeStamps(dbPath, model)
	}
	if res.Errors > 0 {
		return nil
	}
	return stampCurrentSparse(dbPath, model, upgraded, res)
}
