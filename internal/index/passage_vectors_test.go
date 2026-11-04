package index_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/passage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPassageVectors_MigrationAndStoreLifecycle(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "changed"}[changed], func(t *testing.T) {
			db := openTestDB(t)
			rec := index.NoteRecord{ID: "n", Path: "n.md", BodyRaw: "old body", Hash: "old"}
			require.NoError(t, index.StoreNote(db, rec))
			_, err := db.Exec(`INSERT INTO passage_vectors VALUES ('n', 0, 0, 8, ?)`, index.EncodeEmbedding([]float32{1, 0}))
			require.NoError(t, err, "migration 011 must apply")
			if changed {
				rec.BodyRaw, rec.Hash = "new body", "new"
			}
			require.NoError(t, index.StoreNote(db, rec))
			var count int
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM passage_vectors`).Scan(&count))
			assert.Zero(t, count, "every direct store deletes dependent rows, even unchanged stores")
		})
	}
}

type passageRecordingEmbedder struct {
	fakeFullEmbedder
	texts     []string
	fail      bool
	wrongDims bool
	before    func()
}

func (e *passageRecordingEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if e.before != nil {
		e.before()
	}
	if e.fail {
		return nil, errors.New("passage model failed")
	}
	if e.wrongDims {
		return [][]float32{{1}}, nil
	}
	e.texts = append(e.texts, texts...)
	longest := 0
	for _, text := range texts {
		longest = max(longest, len(text))
	}
	if !embedding.BGEM3BatchFits(len(texts), longest) {
		panic("passage batch exceeds BGE-M3 memory bound")
	}
	return e.fakeDenseEmbedder.EmbedBatch(ctx, texts)
}

func TestPassageVectors_EligibilityAndSidecar(t *testing.T) {
	for _, mode := range []string{"short", "comments", "fenced", "sidecar"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "index.db")
			db, err := index.Open(path)
			require.NoError(t, err)
			defer db.Close()
			raw := strings.Repeat("prose ", 120)
			switch mode {
			case "short":
				raw = strings.Repeat("x", 640)
			case "comments":
				raw = "short<!--" + raw + "-->"
			case "fenced":
				raw = "```\n" + raw + "\n```"
			}
			require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "n", Path: "n.md", BodyRaw: raw, BodyText: raw}))
			_, err = db.Exec(`UPDATE notes SET embedding=?, sparse_embedding=?, colbert_embedding=?`, index.EncodeEmbedding([]float32{1, 0, 0, 0}), index.EncodeSparseEmbedding(map[int32]float32{1: 1}), index.EncodeColBERTEmbedding([][]float32{{1, 0}}))
			require.NoError(t, err)
			var emb embedding.Embedder = &passageRecordingEmbedder{fakeFullEmbedder: fakeFullEmbedder{fakeDenseEmbedder{dims: 4}}}
			if mode == "sidecar" {
				emb = &embedding.SidecarBGEM3Embedder{}
			}
			res, err := index.NewIndexer(t.TempDir(), path, nil).EmbedNotes(context.Background(), path, emb)
			require.NoError(t, err)
			assert.Equal(t, 1, res.Skipped)
			vectors, err := index.LoadPassageVectors(db, "n")
			require.NoError(t, err)
			assert.Empty(t, vectors)
		})
	}
}

func TestPassageVectors_FailureIsAtomicAndRetryable(t *testing.T) {
	for _, mode := range []string{"model_error", "dimensions", "changed_source", "storage"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "index.db")
			db, err := index.Open(path)
			require.NoError(t, err)
			defer db.Close()
			raw := strings.Repeat("prose ", 120)
			require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "n", Path: "n.md", BodyRaw: raw, BodyText: raw}))
			emb := &passageRecordingEmbedder{fakeFullEmbedder: fakeFullEmbedder{fakeDenseEmbedder{dims: 4}}}
			switch mode {
			case "model_error":
				emb.fail = true
			case "dimensions":
				emb.wrongDims = true
			case "changed_source":
				emb.before = func() { _, err := db.Exec(`UPDATE notes SET body_raw=body_raw || 'changed'`); require.NoError(t, err) }
			case "storage":
				_, err = db.Exec(`CREATE TRIGGER refuse_passages BEFORE INSERT ON passage_vectors BEGIN SELECT RAISE(ABORT,'refused'); END`)
				require.NoError(t, err)
			}
			_, err = index.NewIndexer(t.TempDir(), path, nil).EmbedNotes(context.Background(), path, emb)
			require.Error(t, err)
			vectors, err := index.LoadPassageVectors(db, "n")
			require.NoError(t, err)
			assert.Empty(t, vectors, "no partial units may survive a failed pass")
			failedBackfill, err := index.NewIndexer(t.TempDir(), path, nil).EmbedNotes(context.Background(), path, emb)
			require.Error(t, err)
			assert.Zero(t, failedBackfill.Skipped, "a failed passage backfill was attempted, not skipped")
			_, err = db.Exec(`DROP TRIGGER IF EXISTS refuse_passages`)
			require.NoError(t, err)
			emb.fail, emb.wrongDims, emb.before = false, false, nil
			res, err := index.NewIndexer(t.TempDir(), path, nil).EmbedNotes(context.Background(), path, emb)
			require.NoError(t, err)
			assert.Equal(t, 1, res.PassageEmbedded)
		})
	}
}

func TestPassageVectors_EmbedAndBackfill(t *testing.T) {
	for _, backfill := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "backfill"}[backfill], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "index.db")
			db, err := index.Open(path)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			units := []string{"Opening words.", "The paraphrased answer lives here.", strings.Repeat("padding ", 100) + "end."}
			raw := "<!-- hidden words -->\n# Title\n\n" + strings.Join(units, "\n\n")
			require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "n", Path: "n.md", Title: "Title", BodyRaw: raw, BodyText: "normalized body"}))
			if backfill {
				_, err = db.Exec(`UPDATE notes SET embedding=?, sparse_embedding=?, colbert_embedding=? WHERE id='n'`, index.EncodeEmbedding([]float32{1, 2, 3, 4}), index.EncodeSparseEmbedding(map[int32]float32{1: 1}), index.EncodeColBERTEmbedding([][]float32{{1, 0}}))
				require.NoError(t, err)
			}
			pending, err := index.PendingEmbeddings(path, embedding.ModelBGEM3)
			require.NoError(t, err)
			assert.Equal(t, 1, pending, "embedded notes without units still need work")
			emb := &passageRecordingEmbedder{fakeFullEmbedder: fakeFullEmbedder{fakeDenseEmbedder{dims: 4}}}
			idx := index.NewIndexer(t.TempDir(), path, nil)
			idx.Progress = io.Discard
			result, err := idx.EmbedNotes(context.Background(), path, emb)
			require.NoError(t, err)
			assert.Zero(t, result.Skipped, "backfilled notes must not be reported skipped")
			assert.Equal(t, 1, result.PassageEmbedded)
			if backfill {
				assert.Zero(t, result.Embedded, "backfill must preserve the note embedding")
				var dense []byte
				require.NoError(t, db.QueryRow(`SELECT embedding FROM notes WHERE id='n'`).Scan(&dense))
				assert.Equal(t, index.EncodeEmbedding([]float32{1, 2, 3, 4}), dense)
			} else {
				assert.Equal(t, 1, result.Embedded)
			}
			assert.ElementsMatch(t, []string{"Title\n\n" + units[0], "Title\n\n" + units[1], "Title\n\n" + units[2]}, emb.texts)
			rows, err := db.Query(`SELECT unit_idx, start, end, dense FROM passage_vectors WHERE note_id='n' ORDER BY unit_idx`)
			require.NoError(t, err)
			defer rows.Close()
			stripped := strings.TrimPrefix(raw, "<!-- hidden words -->")
			i := 0
			for rows.Next() {
				var ordinal, start, end int
				var dense []byte
				require.NoError(t, rows.Scan(&ordinal, &start, &end, &dense))
				require.Less(t, i, len(units))
				assert.Equal(t, i, ordinal)
				assert.Equal(t, units[i], stripped[start:end])
				assert.Len(t, dense, 16)
				i++
			}
			require.NoError(t, rows.Err())
			require.NoError(t, rows.Close())
			assert.Equal(t, len(units), i)
			again, err := idx.EmbedNotes(context.Background(), path, emb)
			require.NoError(t, err)
			assert.Equal(t, 1, again.Skipped)
			assert.Len(t, emb.texts, 3, "rerun must not re-embed units")
		})
	}
}

func TestPassageVectors_MiniLMStoresNone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	db, err := index.Open(path)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "n", Path: "n.md", BodyRaw: strings.Repeat("padding ", 100), BodyText: "body"}))
	_, err = index.NewIndexer(t.TempDir(), path, nil).EmbedNotes(context.Background(), path, fakeDenseEmbedder{dims: 4})
	require.NoError(t, err)
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM passage_vectors`).Scan(&count))
	assert.Zero(t, count)
}

func TestPassageVectors_PlansEveryUnitAndPreservesVectorOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	db, err := index.Open(path)
	require.NoError(t, err)
	defer db.Close()
	var blocks []string
	for i := 1; i <= 19; i++ {
		blocks = append(blocks, strings.Repeat("detail ", i*10)+"end.")
	}
	raw := strings.Join(blocks, "\n\n")
	require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "n", Path: "n.md", Title: "Title", BodyRaw: raw, BodyText: "normalized"}))
	emb := &passageRecordingEmbedder{fakeFullEmbedder: fakeFullEmbedder{fakeDenseEmbedder{dims: 4}}}
	res, err := index.NewIndexer(t.TempDir(), path, nil).EmbedNotes(context.Background(), path, emb)
	require.NoError(t, err)
	assert.Equal(t, 1, res.PassageEmbedded)
	assert.Len(t, emb.texts, 19, "every unit must be embedded despite the eight-text batch cap")
	vectors, err := index.LoadPassageVectors(db, "n")
	require.NoError(t, err)
	units := passage.Units(raw, "Title")
	require.Len(t, vectors, len(units))
	for i, u := range units {
		assert.Equal(t, i, vectors[i].UnitIdx)
		assert.Equal(t, float32(len("Title\n\n"+raw[u.Start:u.End])), vectors[i].Dense[0])
	}
}
