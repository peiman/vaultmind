package index_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingBodyEmbedder struct {
	fakeDenseEmbedder
	texts []string
}

func (e *recordingBodyEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	e.texts = append(e.texts, texts...)
	return e.fakeDenseEmbedder.EmbedBatch(ctx, texts)
}

func TestFaithfulDelivery_UpgradeBackfillsWithoutChangingRetrieval(t *testing.T) {
	for _, touch := range []bool{false, true} {
		t.Run(map[bool]string{false: "mtime fast path", true: "hash fast path"}[touch], func(t *testing.T) {
			root := t.TempDir()
			body := "Use `text_factory` twice: `text_factory`.\n\nUse text\\_factory twice: text\\_factory.\n\nvault_search then last_hidden_state.\n\nUse `*` and `<tmp>`.\n\n```python\ntext_factory = '<tmp>'\nprint(text_factory * 2)\n```\n"
			path := filepath.Join(root, "faithful.md")
			require.NoError(t, os.WriteFile(path, []byte("---\nid: faithful\ntype: concept\ntitle: Faithful\n---\n"+body), 0o600))
			// Empty bodies are valid: distinguish filled-empty from unfilled NULL.
			require.NoError(t, os.WriteFile(filepath.Join(root, "empty.md"), nil, 0o600))
			cfg, err := vault.LoadConfig(testVaultPath)
			require.NoError(t, err)
			dbPath := filepath.Join(root, "index.db")
			idx := index.NewIndexer(root, dbPath, cfg)
			_, err = idx.Incremental()
			require.NoError(t, err)
			emb := &recordingBodyEmbedder{fakeDenseEmbedder: fakeDenseEmbedder{dims: 8}}
			_, err = idx.EmbedNotes(context.Background(), dbPath, emb)
			require.NoError(t, err)
			assert.Equal(t, []string{parser.StripForFTS(body)}, emb.texts)
			db, err := index.Open(dbPath)
			require.NoError(t, err)
			// Retain all three modalities byte-for-byte during the delivery refresh.
			require.NoError(t, index.StoreSparseEmbedding(db, "faithful", map[int32]float32{1: 1}))
			require.NoError(t, index.StoreColBERTEmbedding(db, "faithful", [][]float32{{1, 0}}))
			var dense, sparse, colbert []byte
			var fts string
			var rowid int64
			require.NoError(t, db.QueryRow("SELECT embedding, sparse_embedding, colbert_embedding FROM notes WHERE id='faithful'").Scan(&dense, &sparse, &colbert))
			require.NoError(t, db.QueryRow("SELECT rowid, body_text FROM fts_notes WHERE note_id='faithful'").Scan(&rowid, &fts))
			before, err := index.SearchFTS(db, "Faithful", 10, 0)
			require.NoError(t, err)
			// Recreate the previous schema, then let Open apply migration 009.
			_, err = db.Exec("ALTER TABLE notes DROP COLUMN body_raw")
			require.NoError(t, err)
			_, err = db.Exec("DELETE FROM goose_db_version WHERE version_id=9")
			require.NoError(t, err)
			require.NoError(t, db.Close())
			if touch {
				require.NoError(t, os.Chtimes(path, time.Now().Add(time.Hour), time.Now().Add(time.Hour)))
			}
			res, err := idx.Incremental() // plain index: no embedding pass
			require.NoError(t, err)
			assert.Zero(t, res.Errors)
			db, err = index.Open(dbPath)
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			full, err := db.QueryFullNote("faithful")
			require.NoError(t, err)
			require.NotNil(t, full)
			assert.Equal(t, body, full.Body)
			var d2, s2, c2 []byte
			var f2, normalized string
			var r2 int64
			require.NoError(t, db.QueryRow("SELECT embedding, sparse_embedding, colbert_embedding, body_text FROM notes WHERE id='faithful'").Scan(&d2, &s2, &c2, &normalized))
			require.NoError(t, db.QueryRow("SELECT rowid, body_text FROM fts_notes WHERE note_id='faithful'").Scan(&r2, &f2))
			assert.Equal(t, dense, d2)
			assert.Equal(t, sparse, s2)
			assert.Equal(t, colbert, c2)
			assert.Equal(t, fts, f2)
			assert.Equal(t, rowid, r2)
			assert.Equal(t, parser.StripForFTS(body), normalized)
			after, err := index.SearchFTS(db, "Faithful", 10, 0)
			require.NoError(t, err)
			require.Len(t, after, len(before))
			for i := range before {
				assert.Equal(t, before[i].ID, after[i].ID)
				assert.Equal(t, before[i].Score, after[i].Score)
			}
			var missing int
			require.NoError(t, db.QueryRow("SELECT count(*) FROM notes WHERE body_raw IS NULL").Scan(&missing))
			assert.Zero(t, missing)
			require.NoError(t, db.Close())
			emb.texts = nil
			embedded, err := idx.EmbedNotes(context.Background(), dbPath, emb)
			require.NoError(t, err)
			assert.Zero(t, embedded.Embedded)
			assert.Empty(t, emb.texts)
			again, err := idx.Incremental()
			require.NoError(t, err)
			assert.Equal(t, 2, again.Skipped)
		})
	}
}
