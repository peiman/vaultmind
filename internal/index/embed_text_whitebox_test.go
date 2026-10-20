package index

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/require"
)

func TestEmbedTextFormat_MissingIsZeroAndDenseMakesItStale(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	format, err := ReadEmbedTextFormat(db)
	require.NoError(t, err)
	require.Equal(t, 0, format)
	stale, err := EmbedTextStale(db)
	require.NoError(t, err)
	require.False(t, stale, "a vault with no embeddings is not stale")

	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, embedding) VALUES ('a', 'a.md', 'h', 1, ?)`,
		EncodeEmbedding([]float32{1, 2, 3, 4}))
	require.NoError(t, err)
	stale, err = EmbedTextStale(db)
	require.NoError(t, err)
	require.True(t, stale, "embeddings and no stamp are the old format")

	require.NoError(t, WriteEmbedTextFormat(db, EmbedTextFormatCurrent))
	format, err = ReadEmbedTextFormat(db)
	require.NoError(t, err)
	require.Equal(t, EmbedTextFormatCurrent, format)
	stale, err = EmbedTextStale(db)
	require.NoError(t, err)
	require.False(t, stale)
	require.NoError(t, db.Close())

	stale, err = EmbedTextStaleFile(dbPath)
	require.NoError(t, err)
	require.False(t, stale)
}

func TestReadEmbedTextFormat_ClosedDBAndGarbageValue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	_, err = ReadEmbedTextFormat(db)
	require.Error(t, err)

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`INSERT INTO meta (key, value) VALUES (?, ?)`, metaKeyEmbedTextFormat, "nope")
	require.NoError(t, err)
	_, err = ReadEmbedTextFormat(db)
	require.Error(t, err)
	_, err = EmbedTextStale(db)
	require.Error(t, err)
}

func TestEmbedTextStale_CountFailureAndUnopenableFile(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`ALTER TABLE notes RENAME TO notes_kept`)
	require.NoError(t, err)
	_, err = EmbedTextStale(db)
	require.Error(t, err)

	_, err = EmbedTextStaleFile(blockedDBPath(t))
	require.Error(t, err)
}

type textRecordingFull struct {
	whiteboxSparse
	texts []string
}

func (t *textRecordingFull) EmbedFullBatch(ctx context.Context, texts []string) ([]*embedding.BGEM3Output, error) {
	t.texts = append(t.texts, texts...)
	return t.whiteboxSparse.EmbedFullBatch(ctx, texts)
}

func insertEmbeddedNote(t *testing.T, db *DB, id, title, body string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO notes (id, path, hash, mtime, title, body_text, embedding, sparse_embedding, colbert_embedding)
		VALUES (?, ?, 'h', 1, ?, ?, ?, ?, ?)`,
		id, id+".md", title, body,
		EncodeEmbedding([]float32{9, 9, 9, 9}),
		EncodeSparseEmbedding(map[int32]float32{0: 1}),
		EncodeColBERTEmbedding([][]float32{{8, 8}}))
	require.NoError(t, err)
}

func TestEmbedResolved_UpgradeEmbedTextRewritesEveryNoteAndStampsBoth(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	insertEmbeddedNote(t, db, "a", "Alpha", "alpha body")
	insertEmbeddedNote(t, db, "b", "Beta", "beta body")
	before, _, _ := blobsOf(t, db, "a")
	require.NoError(t, db.Close())

	var buf bytes.Buffer
	idx := NewIndexer(t.TempDir(), dbPath, nil)
	idx.UpgradeEmbedText = true
	idx.Progress = &buf
	script := &textRecordingFull{}
	res, err := idx.embedResolved(context.Background(), dbPath, script, embedding.ModelBGEM3, false)
	require.NoError(t, err)
	require.Equal(t, 2, res.Embedded)
	require.Equal(t, 0, res.Errors)
	require.ElementsMatch(t, []string{"Alpha\n\nalpha body", "Beta\n\nbeta body"}, script.texts)
	require.Contains(t, buf.String(), "re-embedding 2 notes (titles are now embedded)\n")

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	after, sparse, colbert := blobsOf(t, db, "a")
	require.NotEqual(t, before, after, "dense vectors are rewritten in place")
	decoded, err := DecodeSparseEmbedding(sparse)
	require.NoError(t, err)
	require.NotContains(t, decoded, int32(0), "sparse is rewritten by the same pass")
	require.NotEqual(t, EncodeColBERTEmbedding([][]float32{{8, 8}}), colbert)
	text, err := ReadEmbedTextFormat(db)
	require.NoError(t, err)
	require.Equal(t, EmbedTextFormatCurrent, text)
	sparseFormat, err := ReadSparseFormat(db)
	require.NoError(t, err)
	require.Equal(t, SparseFormatCurrent, sparseFormat)
}

func blobsOf(t *testing.T, db *DB, id string) (dense, sparse, colbert []byte) {
	t.Helper()
	require.NoError(t, db.QueryRow(`SELECT embedding, sparse_embedding, colbert_embedding FROM notes WHERE id = ?`, id).Scan(&dense, &sparse, &colbert))
	return dense, sparse, colbert
}

func TestEmbedResolved_WritePathOnStaleVaultEmbedsPendingOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	insertEmbeddedNote(t, db, "old", "Old", "old body")
	// The sparse column is already current, so this pass is only the note
	// write: pending notes, not a sparse migration.
	require.NoError(t, WriteSparseFormat(db, SparseFormatCurrent))
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, title, body_text) VALUES ('new', 'new.md', 'h', 1, 'New', 'new body')`)
	require.NoError(t, err)
	before, _, _ := blobsOf(t, db, "old")
	require.NoError(t, db.Close())

	idx := NewIndexer(t.TempDir(), dbPath, nil)
	script := &textRecordingFull{}
	res, err := idx.embedResolved(context.Background(), dbPath, script, embedding.ModelBGEM3, false)
	require.NoError(t, err)
	require.Equal(t, 1, res.Embedded)
	require.Equal(t, []string{"New\n\nnew body"}, script.texts)

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	after, _, _ := blobsOf(t, db, "old")
	require.Equal(t, before, after, "a note write does not re-embed the vault")
	format, err := ReadEmbedTextFormat(db)
	require.NoError(t, err)
	require.Equal(t, 0, format, "a stale vault is not stamped by a note write")
}

// failCallFull fails one EmbedFullBatch call and then behaves like whiteboxSparse.
type failCallFull struct {
	whiteboxSparse
	failCall int
	call     int
}

func (f *failCallFull) EmbedFullBatch(ctx context.Context, texts []string) ([]*embedding.BGEM3Output, error) {
	n := f.call
	f.call++
	if n == f.failCall {
		return nil, errString("batch failed")
	}
	return f.whiteboxSparse.EmbedFullBatch(ctx, texts)
}

// emptyTextFull returns empty Sparse and ColBERT for one exact embed text.
type emptyTextFull struct {
	whiteboxSparse
	emptyText string
}

func (e *emptyTextFull) EmbedFullBatch(ctx context.Context, texts []string) ([]*embedding.BGEM3Output, error) {
	outs, err := e.whiteboxSparse.EmbedFullBatch(ctx, texts)
	if err != nil {
		return nil, err
	}
	for i, text := range texts {
		if text == e.emptyText {
			outs[i].Sparse = map[int32]float32{}
			outs[i].ColBERT = nil
		}
	}
	return outs, nil
}

type noteVec struct {
	title, body                string
	dense, sparse, colbert     []byte
	hasDense, hasSparse, hasCB bool
}

func scanNoteVecs(t *testing.T, db *DB) map[string]noteVec {
	t.Helper()
	rows, err := db.Query(`SELECT id, COALESCE(title, ''), COALESCE(body_text, ''), embedding, sparse_embedding, colbert_embedding FROM notes`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	out := map[string]noteVec{}
	for rows.Next() {
		var id string
		var nv noteVec
		require.NoError(t, rows.Scan(&id, &nv.title, &nv.body, &nv.dense, &nv.sparse, &nv.colbert))
		nv.hasDense = nv.dense != nil
		nv.hasSparse = nv.sparse != nil
		nv.hasCB = nv.colbert != nil
		out[id] = nv
	}
	require.NoError(t, rows.Err())
	return out
}

func insertFreshNotes(t *testing.T, db *DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("n%02d", i)
		_, err := db.Exec(`INSERT INTO notes (id, path, hash, mtime, title, body_text) VALUES (?, ?, 'h', 1, ?, ?)`,
			id, id+".md", "Title "+id, "body "+id)
		require.NoError(t, err)
	}
}

func insertStaleNotes(t *testing.T, db *DB, n int) {
	t.Helper()
	dense := make([]float32, 1024)
	dense[0] = 9
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("n%02d", i)
		_, err := db.Exec(`INSERT INTO notes (id, path, hash, mtime, title, body_text, embedding, sparse_embedding, colbert_embedding)
			VALUES (?, ?, 'h', 1, ?, ?, ?, ?, ?)`,
			id, id+".md", "Title "+id, "body "+id,
			EncodeEmbedding(dense),
			EncodeSparseEmbedding(map[int32]float32{0: 1}),
			EncodeColBERTEmbedding([][]float32{{8, 8}}))
		require.NoError(t, err)
	}
}

func requireNoVectors(t *testing.T, nv noteVec) {
	t.Helper()
	require.False(t, nv.hasDense, "dense")
	require.False(t, nv.hasSparse, "sparse")
	require.False(t, nv.hasCB, "colbert")
}

func requireAllVectors(t *testing.T, nv noteVec) {
	t.Helper()
	require.True(t, nv.hasDense, "dense")
	require.True(t, nv.hasSparse, "sparse")
	require.True(t, nv.hasCB, "colbert")
}

// A first embed that fails one batch must stamp the text format anyway.
// Otherwise the notes that did embed make the vault look stale, and the next
// index --embed rewrites every note.
func TestEmbedResolved_FreshPartialFailureStampsTextAndRetriesOnlyFailed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	insertFreshNotes(t, db, embedding.BGEM3MaxBatch+1)
	require.NoError(t, db.Close())

	idx := NewIndexer(t.TempDir(), dbPath, nil)
	idx.UpgradeEmbedText = true
	res, err := idx.embedResolved(context.Background(), dbPath, &failCallFull{}, embedding.ModelBGEM3, false)
	require.NoError(t, err)
	require.Greater(t, res.Errors, 0)
	require.Greater(t, res.Embedded, 0)
	require.Equal(t, embedding.BGEM3MaxBatch+1, res.Errors+res.Embedded)

	db, err = Open(dbPath)
	require.NoError(t, err)
	format, err := ReadEmbedTextFormat(db)
	require.NoError(t, err)
	require.Equal(t, EmbedTextFormatCurrent, format)
	var failedTexts []string
	storedDense := map[string][]byte{}
	for id, nv := range scanNoteVecs(t, db) {
		if !nv.hasDense && !nv.hasSparse && !nv.hasCB {
			failedTexts = append(failedTexts, noteEmbedText(nv.title, nv.body))
			continue
		}
		requireAllVectors(t, nv)
		storedDense[id] = append([]byte(nil), nv.dense...)
	}
	require.Len(t, failedTexts, res.Errors)
	require.Len(t, storedDense, res.Embedded)
	require.NoError(t, db.Close())

	var buf bytes.Buffer
	idx2 := NewIndexer(t.TempDir(), dbPath, nil)
	idx2.UpgradeEmbedText = true
	idx2.Progress = &buf
	rec := &textRecordingFull{}
	res2, err := idx2.embedResolved(context.Background(), dbPath, rec, embedding.ModelBGEM3, false)
	require.NoError(t, err)
	require.Equal(t, res.Errors, res2.Embedded, "the next pass embeds the notes that have no vectors")
	require.Equal(t, 0, res2.Errors)
	require.NotContains(t, buf.String(), "titles are now embedded")
	require.GreaterOrEqual(t, len(rec.texts), res2.SparseReembedded)
	require.ElementsMatch(t, failedTexts, rec.texts[res2.SparseReembedded:])

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	for id, nv := range scanNoteVecs(t, db) {
		if before, ok := storedDense[id]; ok {
			require.Equal(t, before, nv.dense, "a note embedded on the first pass is not rewritten")
		}
	}
}

// An upgrade that fails one batch clears that batch and stamps both formats.
// The next upgrade embeds only the cleared notes.
func TestEmbedResolved_UpgradeClearsFailedBatchAndStamps(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	insertStaleNotes(t, db, embedding.BGEM3MaxBatch+1)
	require.NoError(t, db.Close())

	idx := NewIndexer(t.TempDir(), dbPath, nil)
	idx.UpgradeEmbedText = true
	res, err := idx.embedResolved(context.Background(), dbPath, &failCallFull{}, embedding.ModelBGEM3, false)
	require.NoError(t, err)
	require.Greater(t, res.Errors, 0)
	require.Greater(t, res.Embedded, 0)

	db, err = Open(dbPath)
	require.NoError(t, err)
	format, err := ReadEmbedTextFormat(db)
	require.NoError(t, err)
	require.Equal(t, EmbedTextFormatCurrent, format)
	sparseFormat, err := ReadSparseFormat(db)
	require.NoError(t, err)
	require.Equal(t, SparseFormatCurrent, sparseFormat)

	staleDense := make([]float32, 1024)
	staleDense[0] = 9
	staleBlob := EncodeEmbedding(staleDense)
	var clearedTexts []string
	keptDense := map[string][]byte{}
	for id, nv := range scanNoteVecs(t, db) {
		if !nv.hasDense && !nv.hasSparse && !nv.hasCB {
			clearedTexts = append(clearedTexts, noteEmbedText(nv.title, nv.body))
			continue
		}
		requireAllVectors(t, nv)
		require.NotEqual(t, len(staleBlob), len(nv.dense), "kept notes are rewritten from the title")
		keptDense[id] = append([]byte(nil), nv.dense...)
	}
	require.Len(t, clearedTexts, res.Errors)
	require.Len(t, keptDense, res.Embedded)
	require.NoError(t, db.Close())

	idx2 := NewIndexer(t.TempDir(), dbPath, nil)
	idx2.UpgradeEmbedText = true
	rec := &textRecordingFull{}
	res2, err := idx2.embedResolved(context.Background(), dbPath, rec, embedding.ModelBGEM3, false)
	require.NoError(t, err)
	require.Equal(t, res.Errors, res2.Embedded)
	require.Equal(t, 0, res2.SparseReembedded, "the sparse stamp from the upgrade stands")
	require.ElementsMatch(t, clearedTexts, rec.texts)

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	for id, nv := range scanNoteVecs(t, db) {
		if before, ok := keptDense[id]; ok {
			require.Equal(t, before, nv.dense)
		} else {
			requireAllVectors(t, nv)
		}
	}
}

// Empty Sparse or ColBERT on an upgrade drops that note's old vectors.
// The stamps are still written: the note is pending, not a body-only vector.
func TestEmbedResolved_UpgradeClearsEmptyOutputAndStamps(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	insertStaleNotes(t, db, 2)
	require.NoError(t, db.Close())

	db, err = Open(dbPath)
	require.NoError(t, err)
	notes := scanNoteVecs(t, db)
	require.NoError(t, db.Close())
	var emptyID string
	for id := range notes {
		emptyID = id
		break
	}
	emptyText := noteEmbedText(notes[emptyID].title, notes[emptyID].body)

	idx := NewIndexer(t.TempDir(), dbPath, nil)
	idx.UpgradeEmbedText = true
	res, err := idx.embedResolved(context.Background(), dbPath, &emptyTextFull{emptyText: emptyText}, embedding.ModelBGEM3, false)
	require.NoError(t, err)
	require.Equal(t, 1, res.Embedded)
	require.Equal(t, 1, res.EmptyOutput)
	require.Equal(t, 0, res.Errors)

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	format, err := ReadEmbedTextFormat(db)
	require.NoError(t, err)
	require.Equal(t, EmbedTextFormatCurrent, format)
	sparseFormat, err := ReadSparseFormat(db)
	require.NoError(t, err)
	require.Equal(t, SparseFormatCurrent, sparseFormat)
	got := scanNoteVecs(t, db)
	requireNoVectors(t, got[emptyID])
	var kept int
	for id, nv := range got {
		if id == emptyID {
			continue
		}
		requireAllVectors(t, nv)
		kept++
	}
	require.Equal(t, 1, kept)
}

func TestEmbedResolved_CancelledUpgradeDoesNotStamp(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	insertStaleNotes(t, db, 1)
	require.NoError(t, db.Close())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	idx := NewIndexer(t.TempDir(), dbPath, nil)
	idx.UpgradeEmbedText = true
	res, err := idx.embedResolved(ctx, dbPath, &whiteboxSparse{}, embedding.ModelBGEM3, false)
	require.NoError(t, err)
	require.Equal(t, 1, res.Embedded)

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	format, err := ReadEmbedTextFormat(db)
	require.NoError(t, err)
	require.Equal(t, 0, format)
	sparseFormat, err := ReadSparseFormat(db)
	require.NoError(t, err)
	require.Equal(t, 0, sparseFormat)
}

// Every batch can fail. The note's old vectors are cleared and both stamps
// are written: a later index --embed fills the gap, it does not redo the vault.
func TestEmbedResolved_UpgradeAllFailedClearsAndStamps(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	insertEmbeddedNote(t, db, "a", "Alpha", "alpha body")
	require.NoError(t, db.Close())

	idx := NewIndexer(t.TempDir(), dbPath, nil)
	idx.UpgradeEmbedText = true
	script := &textRecordingFull{}
	script.fail = true
	res, err := idx.embedResolved(context.Background(), dbPath, script, embedding.ModelBGEM3, false)
	require.NoError(t, err)
	require.Greater(t, res.Errors, 0)
	require.Equal(t, 0, res.Embedded)

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	requireNoVectors(t, scanNoteVecs(t, db)["a"])
	format, err := ReadEmbedTextFormat(db)
	require.NoError(t, err)
	require.Equal(t, EmbedTextFormatCurrent, format)
	sparseFormat, err := ReadSparseFormat(db)
	require.NoError(t, err)
	require.Equal(t, SparseFormatCurrent, sparseFormat)
}

func TestEmbedResolved_FreshVaultWritesEmbedTextStamp(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, title, body_text) VALUES ('a', 'a.md', 'h', 1, 'Alpha', 'alpha body')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	idx := NewIndexer(t.TempDir(), dbPath, nil)
	idx.UpgradeEmbedText = true
	res, err := idx.embedResolved(context.Background(), dbPath, &whiteboxSparse{}, embedding.ModelBGEM3, false)
	require.NoError(t, err)
	require.Equal(t, 1, res.Embedded)

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	format, err := ReadEmbedTextFormat(db)
	require.NoError(t, err)
	require.Equal(t, EmbedTextFormatCurrent, format)
}

func TestEmbedResolved_FullStampsBothFormats(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	insertEmbeddedNote(t, db, "a", "Alpha", "alpha body")
	require.NoError(t, db.Close())

	idx := NewIndexer(t.TempDir(), dbPath, nil)
	idx.UpgradeEmbedText = true
	res, err := idx.embedResolved(context.Background(), dbPath, &whiteboxSparse{}, embedding.ModelBGEM3, true)
	require.NoError(t, err)
	require.Equal(t, 1, res.Embedded)

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	format, err := ReadEmbedTextFormat(db)
	require.NoError(t, err)
	require.Equal(t, EmbedTextFormatCurrent, format)
	sparseFormat, err := ReadSparseFormat(db)
	require.NoError(t, err)
	require.Equal(t, SparseFormatCurrent, sparseFormat)
}

func TestSkipIncrementalEmbed_CurrentEmbedTextStampIsIdle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, embedding) VALUES ('m', 'm.md', 'h', 1, ?)`,
		EncodeEmbedding(make([]float32, 384)))
	require.NoError(t, err)
	require.NoError(t, WriteEmbedTextFormat(db, EmbedTextFormatCurrent))
	require.NoError(t, db.Close())

	skip, res, err := skipIncrementalEmbed(dbPath, embedding.ModelMiniLM, true)
	require.NoError(t, err)
	require.True(t, skip, "a current stamp and nothing pending skips the model load")
	require.Equal(t, 1, res.Skipped)
}

func TestSkipIncrementalEmbed_StaleEmbedTextIsNotIdle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, embedding) VALUES ('m', 'm.md', 'h', 1, ?)`,
		EncodeEmbedding(make([]float32, 384)))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	skip, _, err := skipIncrementalEmbed(dbPath, embedding.ModelMiniLM, true)
	require.NoError(t, err)
	require.False(t, skip, "titles still have to be embedded")

	skip, _, err = skipIncrementalEmbed(dbPath, embedding.ModelMiniLM, false)
	require.NoError(t, err)
	require.True(t, skip, "a note write does not load the model when nothing is pending")
}
