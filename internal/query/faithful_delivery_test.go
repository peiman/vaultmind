package query_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/navigate"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFaithfulDelivery(t *testing.T) {
	fixtures := map[string]string{
		"repeated inline identifier": "Use `text_factory` twice: `text_factory`.",
		"escaped identifier":         `Use text\_factory twice: text\_factory.`,
		"two paragraphs":             "Use vault_search to find the note.\n\nRead last_hidden_state from the result.",
		"inline symbols":             "Use `*` and `<tmp>` in the expression.",
		"fenced example":             "```python\ntext_factory = '<tmp>'\nprint(text_factory * 2)\n```",
	}
	for name, body := range fixtures {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			// Principle selection exercises ask's excerpt path, including paragraph
			// breaks and code indentation, with enough budget to retain the passage.
			raw := "# Principle\n\n" + body + "\n\n# Source\n\nProvenance.\n"
			require.NoError(t, os.WriteFile(filepath.Join(root, "faithful.md"), []byte("---\nid: faithful\ntype: concept\ntitle: Faithful\npaths: ['**']\n---\n"+raw), 0o600))
			cfg, err := vault.LoadConfig(testVaultPath)
			require.NoError(t, err)
			dbPath := filepath.Join(root, "index.db")
			_, err = index.NewIndexer(root, dbPath, cfg).Incremental()
			require.NoError(t, err)
			db, err := index.Open(dbPath)
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })

			notes, err := navigate.Load(db, navigate.Filter{})
			require.NoError(t, err)
			require.Len(t, notes, 1)
			assert.Equal(t, navigate.OneLine(raw), notes[0].Line)
			covering, err := navigate.Covering(db, navigate.CodeFile{Rel: "any.go"})
			require.NoError(t, err)
			require.Len(t, covering, 1)
			assert.Equal(t, notes[0].Line, covering[0].Line)
			require.NoError(t, index.StoreEmbedding(db, "faithful", []float32{1, 0}))
			require.NoError(t, index.StoreSparseEmbedding(db, "faithful", map[int32]float32{1: 1}))
			require.NoError(t, index.StoreColBERTEmbedding(db, "faithful", [][]float32{{1, 0}}))
			retrievers := []retrieval.Retriever{
				&query.FTSRetriever{DB: db},
				&query.EmbeddingRetriever{DB: db, Embedder: &mockEmbedder{vec: []float32{1, 0}, dims: 2}},
				&query.SparseRetriever{DB: db, EmbedSparse: func(context.Context, string) (map[int32]float32, error) { return map[int32]float32{1: 1}, nil }},
				&query.ColBERTRetriever{DB: db, Dims: 2, EmbedColBERT: func(context.Context, string) ([][]float32, error) { return [][]float32{{1, 0}}, nil }},
			}
			for _, r := range retrievers {
				hits, _, err := r.Search(context.Background(), "Faithful", 5, 0, index.SearchFilters{})
				require.NoError(t, err)
				require.Len(t, hits, 1)
				assert.Equal(t, raw, hits[0].Snippet)
			}

			for _, jsonOutput := range []bool{false, true} {
				var out bytes.Buffer
				_, err = query.RunNoteGet(db, query.NoteGetConfig{Input: "faithful", JSONOutput: jsonOutput}, &out)
				require.NoError(t, err)
				if jsonOutput {
					var env struct {
						Result struct {
							Body string `json:"body"`
						} `json:"result"`
					}
					require.NoError(t, json.Unmarshal(out.Bytes(), &env))
					assert.Equal(t, raw, env.Result.Body)
				} else {
					assert.Contains(t, out.String(), "\n"+raw+"\n")
				}
			}
			result, err := query.Ask(context.Background(), &query.FTSRetriever{DB: db}, graph.NewResolver(db), db, query.AskConfig{
				Query: "Faithful", Budget: 4000, MaxItems: 8, SearchLimit: 5, ExcerptTokens: 1000,
			})
			require.NoError(t, err)
			require.NotNil(t, result.Context)
			require.NotNil(t, result.Context.Target)
			assert.True(t, result.Context.Target.BodyExcerpted)
			assert.Equal(t, body, result.Context.Target.Body)
		})
	}
}

// A preview read failure must be reported after scoring, rather than making
// the scoring loads depend on the delivery column.
func TestFaithfulDelivery_PreviewReadFailure(t *testing.T) {
	db := buildIndexedDB(t)
	require.NoError(t, index.StoreEmbedding(db, "concept-act-r", []float32{1, 0}))
	_, err := db.Exec("ALTER TABLE notes DROP COLUMN body_raw")
	require.NoError(t, err)
	r := &query.EmbeddingRetriever{DB: db, Embedder: &mockEmbedder{vec: []float32{1, 0}, dims: 2}}
	_, _, err = r.Search(t.Context(), "memory", 5, 0, index.SearchFilters{})
	require.ErrorContains(t, err, "reading raw body preview")
}
