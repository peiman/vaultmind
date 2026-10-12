package query_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/navigate"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBodyRawFallback_AllReadersNullOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  any
		want string
	}{
		{"NULL legacy body", nil, "Legacy body sentence. More."},
		{"intentionally empty", "", ""},
		{"raw preferred", "As written body.", "As written body."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			_, err = db.Exec(`INSERT INTO notes (id,path,title,type,body_text,body_raw,hash,mtime) VALUES ('legacy','legacy.md','Legacy','concept','Legacy body sentence. More.',?,'h',1)`, tc.raw)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO fts_notes (note_id,title,body_text) VALUES ('legacy','Legacy','Legacy body sentence. More.')`)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO frontmatter_kv (note_id,key,value_json) VALUES ('legacy','paths','["**"]')`)
			require.NoError(t, err)
			full, err := db.QueryFullNote("legacy")
			require.NoError(t, err)
			assert.Equal(t, tc.want, full.Body)
			hits, err := index.SearchFTS(db, "Legacy", 5, 0)
			require.NoError(t, err)
			require.Len(t, hits, 1)
			assert.Equal(t, tc.want, hits[0].Snippet)
			require.NoError(t, index.StoreEmbedding(db, "legacy", []float32{1, 0}))
			require.NoError(t, index.StoreSparseEmbedding(db, "legacy", map[int32]float32{1: 1}))
			require.NoError(t, index.StoreColBERTEmbedding(db, "legacy", [][]float32{{1, 0}}))
			readers := []retrieval.Retriever{
				&query.FTSRetriever{DB: db},
				&query.EmbeddingRetriever{DB: db, Embedder: &mockEmbedder{vec: []float32{1, 0}, dims: 2}},
				&query.SparseRetriever{DB: db, EmbedSparse: func(context.Context, string) (map[int32]float32, error) { return map[int32]float32{1: 1}, nil }},
				&query.ColBERTRetriever{DB: db, Dims: 2, EmbedColBERT: func(context.Context, string) ([][]float32, error) { return [][]float32{{1, 0}}, nil }},
			}
			for _, r := range readers {
				hits, _, err := r.Search(t.Context(), "Legacy", 5, 0, index.SearchFilters{})
				require.NoError(t, err)
				require.Len(t, hits, 1)
				assert.Equal(t, tc.want, hits[0].Snippet)
			}
			notes, err := navigate.Load(db, navigate.Filter{})
			require.NoError(t, err)
			require.Len(t, notes, 1)
			assert.Equal(t, navigate.OneLine(tc.want), notes[0].Line)
			notes, err = navigate.Covering(db, navigate.CodeFile{Rel: "any.go"})
			require.NoError(t, err)
			require.Len(t, notes, 1)
			assert.Equal(t, navigate.OneLine(tc.want), notes[0].Line)
			// Excerpting engages the separate passage reader after packing.
			result, err := query.Ask(t.Context(), &query.FTSRetriever{DB: db}, graph.NewResolver(db), db, query.AskConfig{Query: "Legacy", Budget: 900, MaxItems: 3, SearchLimit: 5, ExcerptTokens: 160})
			require.NoError(t, err)
			require.NotNil(t, result.Context)
			assert.Equal(t, tc.want, result.Context.Target.Body)
		})
	}
}

func TestBodyRawFallback_FooterCauses(t *testing.T) {
	for _, maxItems := range []int{0, 3} {
		for _, tc := range []struct {
			name, target, neighbor  string
			budget                  int
			missing, budgetOmission bool
		}{
			{"missing target", "", "", 900, true, false},
			{"empty neighbor", "Target body.", "", 900, true, false},
			{"budget neighbor", "Target body.", strings.Repeat("large body ", 1000), 900, false, true},
			{"both causes", "", strings.Repeat("large body ", 1000), 900, true, true},
			{"budget target", "Target body.", "", 1, false, true},
		} {
			t.Run(fmt.Sprintf("%s/max-items=%d", tc.name, maxItems), func(t *testing.T) {
				db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
				require.NoError(t, err)
				t.Cleanup(func() { _ = db.Close() })
				for id, body := range map[string]string{"target": tc.target, "neighbor": tc.neighbor} {
					_, err = db.Exec(`INSERT INTO notes (id,path,title,type,body_text,body_raw,hash,mtime) VALUES (?,?,?,'concept',?,?,'h',1)`, id, id+".md", id, body, body)
					require.NoError(t, err)
				}
				pack, err := memory.ContextPack(graph.NewResolver(db), db, memory.ContextPackConfig{Input: "target", Budget: tc.budget, MaxItems: maxItems, Seeds: []memory.Seed{{ID: "neighbor"}}})
				require.NoError(t, err)
				var out strings.Builder
				require.NoError(t, query.FormatAsk(&query.AskResult{Query: "target", Context: pack}, &out))
				// These rows store body_raw as a string, including "". That is an
				// indexed empty body, not an unfilled NULL, so re-indexing
				// cannot fill it.
				assert.Equal(t, tc.missing, strings.Contains(out.String(), "no body text"), out.String())
				assert.NotContains(t, out.String(), "vaultmind index", out.String())
				assert.Equal(t, tc.budgetOmission, strings.Contains(out.String(), "no room"), out.String())
			})
		}
	}
}

func askFooter(t *testing.T, db *index.DB, input string, seeds ...memory.Seed) string {
	t.Helper()
	pack, err := memory.ContextPack(graph.NewResolver(db), db, memory.ContextPackConfig{Input: input, Budget: 900, MaxItems: 3, Seeds: seeds})
	require.NoError(t, err)
	var out strings.Builder
	require.NoError(t, query.FormatAsk(&query.AskResult{Query: input, Context: pack}, &out))
	return out.String()
}

func openFooterDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// A stored empty body, including a note that is only frontmatter, has nothing
// for `vaultmind index` to fill. The footer must say so and then stop.
func TestBodyRawFallback_EmptyNoteFooter(t *testing.T) {
	t.Run("stored empty body", func(t *testing.T) {
		db := openFooterDB(t)
		_, err := db.Exec(`INSERT INTO notes (id,path,title,type,body_text,body_raw,hash,mtime) VALUES ('bare','bare.md','Bare','concept','','','h',1)`)
		require.NoError(t, err)
		out := askFooter(t, db, "bare")
		assert.Contains(t, out, "note has no body text", out)
		assert.NotContains(t, out, "vaultmind index", out)
		assert.NotContains(t, out, "re-index", out)
	})
	t.Run("frontmatter only", func(t *testing.T) {
		parsed, err := parser.Parse([]byte("---\nid: bare\ntitle: Bare\ntype: concept\n---\n"))
		require.NoError(t, err)
		require.Empty(t, parsed.Body, "a frontmatter-only note has no body to store")
		db := openFooterDB(t)
		require.NoError(t, index.StoreNote(db, index.NoteRecord{
			ID: parsed.ID, Path: "bare.md", Title: "Bare", Type: parsed.NoteType,
			BodyRaw: parsed.Body, BodyText: parsed.FTSBody,
		}))
		out := askFooter(t, db, parsed.ID)
		assert.Contains(t, out, "note has no body text", out)
		assert.NotContains(t, out, "vaultmind index", out)
		assert.NotContains(t, out, "re-index", out)
	})
}

// body_raw NULL is the unfilled migration state. Nothing was stored, so
// re-indexing can still copy the file. An empty string must not take this path.
func TestBodyRawFallback_NullRawFooterAdvisesReindex(t *testing.T) {
	db := openFooterDB(t)
	_, err := db.Exec(`INSERT INTO notes (id,path,title,type,body_text,body_raw,hash,mtime) VALUES ('legacy','legacy.md','Legacy','concept','',NULL,'h',1)`)
	require.NoError(t, err)
	out := askFooter(t, db, "legacy")
	assert.Contains(t, out, "no body content available — re-index with `vaultmind index` to fill it", out)
	assert.NotContains(t, out, "no body text", out)
}

// A short-body assertion alone cannot catch a failed passage read: ask keeps
// its first excerpt on read errors. Require a matching later passage to arrive.
func TestBodyRawFallback_LatePassage(t *testing.T) {
	db := passageDB(t, passageParagraph("intro", 74)+"\n\n"+passageParagraph("beta gamma", 73))
	_, err := db.Exec("UPDATE notes SET body_raw=NULL WHERE id='target'")
	require.NoError(t, err)
	result, err := query.Ask(t.Context(), &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Title: "Passage title", Score: 1}}}, graph.NewResolver(db), db, query.AskConfig{Query: "beta gamma", Budget: 1500, MaxItems: 3, SearchLimit: 5, ExcerptTokens: 160})
	require.NoError(t, err)
	require.NotNil(t, result.Context)
	assert.Contains(t, result.Context.Target.Body, "beta gamma")
}

func TestBodyRawFallback_DoctorReadError(t *testing.T) {
	db := buildIndexedDB(t)
	_, err := db.Exec("ALTER TABLE notes DROP COLUMN body_raw")
	require.NoError(t, err)
	_, err = query.Doctor(db, testVaultPath, nil)
	require.ErrorContains(t, err, "counting notes without raw bodies")
}
