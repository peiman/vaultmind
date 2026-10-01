package index_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingFullEmbedder is a BGE-M3 stand-in that remembers every text it was
// given. A section under a heading starting "EMPTY" gets no sparse terms —
// the shape the real heads sometimes produce (vaultmind#22).
type recordingFullEmbedder struct {
	fakeDenseEmbedder
	mu    *sync.Mutex
	texts *[]string
}

func newRecordingFullEmbedder() recordingFullEmbedder {
	return recordingFullEmbedder{fakeDenseEmbedder: fakeDenseEmbedder{dims: 4}, mu: &sync.Mutex{}, texts: &[]string{}}
}

func (f recordingFullEmbedder) EmbedFullBatch(_ context.Context, texts []string) ([]*embedding.BGEM3Output, error) {
	f.mu.Lock()
	*f.texts = append(*f.texts, texts...)
	f.mu.Unlock()
	out := make([]*embedding.BGEM3Output, len(texts))
	for i, tx := range texts {
		sparse := map[int32]float32{1: 0.5}
		// Only a section's text has the "title › heading" line, so this hits
		// the section and not the note whose body also says "EMPTY".
		if strings.Contains(tx, "› EMPTY") {
			sparse = map[int32]float32{}
		}
		out[i] = &embedding.BGEM3Output{Dense: []float32{1, 0, 0, 0}, Sparse: sparse, ColBERT: [][]float32{{0.1, 0.2}}}
	}
	return out, nil
}

func (f recordingFullEmbedder) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), *f.texts...)
}

// longNoteMarkdown is a note past the model's window with level-2 headings.
func longNoteMarkdown(title string, headings ...string) string {
	var b strings.Builder
	b.WriteString("---\nid: ref-long\ntype: concept\ntitle: " + title + "\n---\n")
	for _, h := range headings {
		b.WriteString("## " + h + "\n\n" + prose(1500))
	}
	for b.Len() < 9000*4 {
		b.WriteString(prose(1500))
	}
	return b.String()
}

// buildLongNoteVault is buildEmbedTestVault plus one long note.
func buildLongNoteVault(t *testing.T, headings ...string) (vaultRoot, dbPath string, idxr *index.Indexer) {
	t.Helper()
	vaultRoot, dbPath = buildEmbedTestVault(t)
	require.NoError(t, os.WriteFile(filepath.Join(vaultRoot, "long.md"), []byte(longNoteMarkdown("Long Guide", headings...)), 0o644))
	cfg, err := vault.LoadConfig(vaultRoot)
	require.NoError(t, err)
	idxr = index.NewIndexer(vaultRoot, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)
	return vaultRoot, dbPath, idxr
}

func openDB(t *testing.T, dbPath string) *index.DB {
	t.Helper()
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func sectionEmbeddingCounts(t *testing.T, dbPath string) (total, dense, sparse int) {
	t.Helper()
	db := openDB(t, dbPath)
	require.NoError(t, db.QueryRow(`SELECT COUNT(*), COUNT(embedding), COUNT(sparse_embedding) FROM sections`).Scan(&total, &dense, &sparse))
	return total, dense, sparse
}

// The embed pass embeds each section of a long note, dense and sparse, from
// its text with the note title and heading path in front — and a second pass
// re-embeds nothing.
func TestEmbedNotes_EmbedsTheSectionsOfALongNote(t *testing.T) {
	_, dbPath, idxr := buildLongNoteVault(t, "Install", "Configure", "Troubleshoot")
	emb := newRecordingFullEmbedder()

	r1, err := idxr.EmbedNotes(context.Background(), dbPath, emb)
	require.NoError(t, err)
	total, dense, sparse := sectionEmbeddingCounts(t, dbPath)
	require.GreaterOrEqual(t, total, 3)
	assert.Equal(t, total, dense)
	assert.Equal(t, total, sparse)
	assert.Equal(t, total, r1.SectionsEmbedded)

	var prefixed int
	for _, tx := range emb.seen() {
		if strings.HasPrefix(tx, "Long Guide › Configure\n\n") {
			prefixed++
		}
	}
	assert.Equal(t, 1, prefixed, "a section is embedded once, behind its title › heading line")

	r2, err := idxr.EmbedNotes(context.Background(), dbPath, emb)
	require.NoError(t, err)
	assert.Zero(t, r2.SectionsEmbedded, "nothing re-embedded on the second pass")
}

// A dense-only model (MiniLM) stores dense vectors for sections and no sparse.
func TestEmbedNotes_DenseOnlyModelEmbedsSectionsDense(t *testing.T) {
	_, dbPath, idxr := buildLongNoteVault(t, "Install", "Configure")
	r, err := idxr.EmbedNotes(context.Background(), dbPath, fakeDenseEmbedder{dims: 8})
	require.NoError(t, err)
	total, dense, sparse := sectionEmbeddingCounts(t, dbPath)
	assert.Equal(t, total, dense)
	assert.Zero(t, sparse)
	assert.Equal(t, total, r.SectionsEmbedded)
}

// Sections a pre-sections binary left behind (their note was re-indexed, so
// the hashes differ) are not embedded: they are about to be replaced.
func TestEmbedNotes_SkipsStaleSections(t *testing.T) {
	_, dbPath, idxr := buildLongNoteVault(t, "Install", "Configure")
	_, err := openDB(t, dbPath).Exec(`UPDATE notes SET hash = 'edited-by-an-older-binary' WHERE id = 'ref-long'`)
	require.NoError(t, err)

	r, err := idxr.EmbedNotes(context.Background(), dbPath, newRecordingFullEmbedder())
	require.NoError(t, err)
	assert.Zero(t, r.SectionsEmbedded)
	_, dense, _ := sectionEmbeddingCounts(t, dbPath)
	assert.Zero(t, dense)
}

// A section whose sparse output comes back empty is not stored half-done: it
// stays pending for the next pass, and the result says so.
func TestEmbedNotes_AnEmptySectionOutputStaysPending(t *testing.T) {
	_, dbPath, idxr := buildLongNoteVault(t, "EMPTY heads", "Configure")
	r, err := idxr.EmbedNotes(context.Background(), dbPath, newRecordingFullEmbedder())
	require.NoError(t, err)
	total, dense, _ := sectionEmbeddingCounts(t, dbPath)
	assert.Equal(t, total-1, dense, "the empty one is left unembedded")
	assert.Equal(t, 1, r.SectionsEmptyOutput)
	pending, err := index.PendingEmbeddings(dbPath, embedding.ModelBGEM3)
	require.NoError(t, err)
	assert.Equal(t, 1, pending)
}

// An existing vault whose notes are all embedded still has work when its long
// notes gain sections: the pending count includes them, or the embed command
// would decide there is nothing to do and never load the model.
func TestPendingEmbeddings_CountsSections(t *testing.T) {
	_, dbPath, idxr := buildLongNoteVault(t, "Install", "Configure")
	_, err := idxr.EmbedNotes(context.Background(), dbPath, newRecordingFullEmbedder())
	require.NoError(t, err)
	pending, err := index.PendingEmbeddings(dbPath, embedding.ModelBGEM3)
	require.NoError(t, err)
	require.Zero(t, pending)

	_, err = openDB(t, dbPath).Exec(`UPDATE sections SET embedding = NULL, sparse_embedding = NULL`)
	require.NoError(t, err)
	total, _, _ := sectionEmbeddingCounts(t, dbPath)
	pending, err = index.PendingEmbeddings(dbPath, embedding.ModelBGEM3)
	require.NoError(t, err)
	assert.Equal(t, total, pending)
}

// Switching models purges every vector — sections too, or the index would mix
// two models' sections with the new model's notes.
func TestPurgeEmbeddings_ClearsSections(t *testing.T) {
	_, dbPath, idxr := buildLongNoteVault(t, "Install", "Configure")
	_, err := idxr.EmbedNotes(context.Background(), dbPath, newRecordingFullEmbedder())
	require.NoError(t, err)
	_, err = index.PurgeEmbeddings(dbPath)
	require.NoError(t, err)
	_, dense, sparse := sectionEmbeddingCounts(t, dbPath)
	assert.Zero(t, dense)
	assert.Zero(t, sparse)
}

// A vault of short notes embeds exactly as before: the same texts, no section
// work.
func TestEmbedNotes_ShortNotesSendNoSectionTexts(t *testing.T) {
	vaultRoot, dbPath := buildEmbedTestVault(t)
	cfg, err := vault.LoadConfig(vaultRoot)
	require.NoError(t, err)
	emb := newRecordingFullEmbedder()
	r, err := index.NewIndexer(vaultRoot, dbPath, cfg).EmbedNotes(context.Background(), dbPath, emb)
	require.NoError(t, err)
	assert.Zero(t, r.SectionsEmbedded)
	assert.Len(t, emb.seen(), 3, "one text per note, nothing else")
}

// sectionFailingEmbedder fails any batch that holds a section's text (only
// those carry the "title › heading" line), so notes embed and sections fail.
type sectionFailingEmbedder struct {
	recordingFullEmbedder
}

func (f sectionFailingEmbedder) EmbedFullBatch(ctx context.Context, texts []string) ([]*embedding.BGEM3Output, error) {
	for _, tx := range texts {
		if strings.Contains(tx, " › ") {
			return nil, errors.New("model failed")
		}
	}
	return f.recordingFullEmbedder.EmbedFullBatch(ctx, texts)
}

func (f sectionFailingEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	for _, tx := range texts {
		if strings.Contains(tx, " › ") {
			return nil, errors.New("model failed")
		}
	}
	return f.fakeDenseEmbedder.EmbedBatch(ctx, texts)
}

// denseOnly hides EmbedFullBatch so the dense (MiniLM) path is taken.
type denseOnly struct{ e sectionFailingEmbedder }

func (d denseOnly) Embed(ctx context.Context, t string) ([]float32, error) { return d.e.Embed(ctx, t) }
func (d denseOnly) EmbedBatch(ctx context.Context, t []string) ([][]float32, error) {
	return d.e.EmbedBatch(ctx, t)
}
func (d denseOnly) Dims() int    { return d.e.Dims() }
func (d denseOnly) Close() error { return nil }

// A failed model call on a batch of sections counts them as errors and leaves
// them pending for the next pass — with either kind of model.
func TestEmbedNotes_AFailedSectionBatchStaysPending(t *testing.T) {
	for name, emb := range map[string]embedding.Embedder{
		"bge-m3": sectionFailingEmbedder{recordingFullEmbedder: newRecordingFullEmbedder()},
		"minilm": denseOnly{e: sectionFailingEmbedder{recordingFullEmbedder: newRecordingFullEmbedder()}},
	} {
		t.Run(name, func(t *testing.T) {
			_, dbPath, idxr := buildLongNoteVault(t, "Install", "Configure")
			r, err := idxr.EmbedNotes(context.Background(), dbPath, emb)
			require.NoError(t, err)
			total, dense, _ := sectionEmbeddingCounts(t, dbPath)
			assert.Zero(t, dense)
			assert.Equal(t, total, r.SectionsErrors)
			assert.Zero(t, r.SectionsEmbedded)
		})
	}
}

// A batch whose store fails is stored not at all: all or nothing, counted as
// errors, still pending.
func TestEmbedNotes_AFailedSectionStoreIsAllOrNothing(t *testing.T) {
	_, dbPath, idxr := buildLongNoteVault(t, "Install", "Configure")
	_, err := openDB(t, dbPath).Exec(`CREATE TRIGGER refuse_section_vectors BEFORE UPDATE ON sections
		BEGIN SELECT RAISE(ABORT, 'refused'); END`)
	require.NoError(t, err)
	r, err := idxr.EmbedNotes(context.Background(), dbPath, newRecordingFullEmbedder())
	require.NoError(t, err)
	total, dense, _ := sectionEmbeddingCounts(t, dbPath)
	assert.Zero(t, dense)
	assert.Equal(t, total, r.SectionsErrors)
}

// A broken sections table is an error from the count and the purge, not a
// silent "nothing pending" or a half-done purge reported as done.
func TestSectionCountAndPurge_ReportABrokenTable(t *testing.T) {
	_, dbPath, _ := buildLongNoteVault(t, "Install", "Configure")
	_, err := openDB(t, dbPath).Exec(`DROP TABLE sections`)
	require.NoError(t, err)
	_, err = index.PendingEmbeddings(dbPath, embedding.ModelBGEM3)
	assert.ErrorContains(t, err, "sections")
	_, err = index.PurgeEmbeddings(dbPath)
	assert.ErrorContains(t, err, "section")
}
