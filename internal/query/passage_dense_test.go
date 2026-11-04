package query_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/passage"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Models the production query memo: callers sharing the exact input share
// one forward. A changed ranking string would cost another forward here.
type passageMemoEmbedder struct {
	texts    []string
	seen     map[string]bool
	forwards int
}

func (e *passageMemoEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.texts = append(e.texts, text)
	if e.seen == nil {
		e.seen = map[string]bool{}
	}
	if !e.seen[text] {
		e.forwards++
		e.seen[text] = true
	}
	return []float32{1, 0}, nil
}
func (e *passageMemoEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	var out [][]float32
	for _, text := range texts {
		v, _ := e.Embed(ctx, text)
		out = append(out, v)
	}
	return out, nil
}
func (*passageMemoEmbedder) Dims() int    { return 2 }
func (*passageMemoEmbedder) Close() error { return nil }

func passageSelector(t *testing.T, target *memory.ContextPackTarget) string {
	t.Helper()
	encoded, err := json.Marshal(target)
	require.NoError(t, err)
	var fields map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &fields))
	selector, _ := fields["passage_selector"].(string)
	return selector
}

func densePassageFixture(t *testing.T) (*index.DB, string) {
	t.Helper()
	intro := passageParagraph("alpha", 100)
	texts := []string{intro, "Use a stable snapshot.", "Preserve the immutable receipt.", "An irrelevant distraction.", "Verify the restored state."}
	body := "# Passage title\n\n" + intro + "\n\n## Answer\n\n" + texts[1] + " " + texts[2] + "\n\n" + texts[3] + "\n\n## Final\n\n" + texts[4]
	db := passageDB(t, body)
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS passage_vectors (note_id TEXT NOT NULL REFERENCES notes(id), unit_idx INTEGER NOT NULL, start INTEGER NOT NULL, end INTEGER NOT NULL, dense BLOB NOT NULL, PRIMARY KEY(note_id,unit_idx))`)
	require.NoError(t, err)
	for i, text := range texts {
		start := strings.Index(body, text)
		x := []float32{0, 0.8, 1, 0, 0.9}[i]
		_, err = db.Exec(`INSERT INTO passage_vectors VALUES ('target', ?, ?, ?, ?)`, i, start, start+len(text), index.EncodeEmbedding([]float32{x, 1 - x}))
		require.NoError(t, err)
	}
	return db, body
}

func askDensePassage(t *testing.T, db *index.DB, emb *passageMemoEmbedder, excerpt int) *query.AskResult {
	t.Helper()
	r, err := query.Ask(context.Background(), &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Title: "Passage title", Score: 1}}}, graph.NewResolver(db), db, query.AskConfig{Query: "alpha " + strings.Repeat(".", 40), Embedder: emb, Budget: 1500, MaxItems: 3, SearchLimit: 5, ExcerptTokens: excerpt})
	require.NoError(t, err)
	require.NotNil(t, r.Context)
	require.NotNil(t, r.Context.Target)
	return r
}

func TestAsk_DensePassageTopThree(t *testing.T) {
	db, _ := densePassageFixture(t)
	emb := &passageMemoEmbedder{}
	r := askDensePassage(t, db, emb, 160)
	assert.Equal(t, "## Answer\n\nUse a stable snapshot. Preserve the immutable receipt.\n…\n## Final\n\nVerify the restored state.", r.Context.Target.Body)
	assert.Equal(t, "dense", passageSelector(t, r.Context.Target))
	assert.Equal(t, 1, emb.forwards, "passages and NoteSimilarities must share the ranking query memo")
	require.Len(t, emb.texts, 2)
	assert.Equal(t, emb.texts[0], emb.texts[1])
	assert.NotContains(t, emb.texts[1], strings.Repeat(".", 40), "use the shaped ranking query")
}

func TestAsk_DensePassageSpanMismatch(t *testing.T) {
	db, _ := densePassageFixture(t)
	_, err := db.Exec(`UPDATE passage_vectors SET start=start+1 WHERE unit_idx=2`)
	require.NoError(t, err)
	r := askDensePassage(t, db, &passageMemoEmbedder{}, 160)
	assert.Contains(t, r.Context.Target.Body, "alpha")
	assert.NotContains(t, r.Context.Target.Body, "immutable receipt")
	assert.Equal(t, "lexical", passageSelector(t, r.Context.Target))
}

func TestAsk_PassageSelectorFallbacks(t *testing.T) {
	for _, mode := range []string{"dimensions", "missing", "principle", "none"} {
		t.Run(mode, func(t *testing.T) {
			db, body := densePassageFixture(t)
			excerpt := 160
			want := "lexical"
			switch mode {
			case "dimensions":
				_, err := db.Exec(`UPDATE passage_vectors SET dense=?`, index.EncodeEmbedding([]float32{1, 0, 0}))
				require.NoError(t, err)
			case "missing":
				_, err := db.Exec(`DELETE FROM passage_vectors`)
				require.NoError(t, err)
			case "principle":
				_, err := db.Exec(`UPDATE notes SET body_raw=? WHERE id='target'`, body+"\n\n## Principle\n\nKeep today's rule.")
				require.NoError(t, err)
				want = "principle"
			case "none":
				excerpt, want = 0, "none"
			}
			r := askDensePassage(t, db, &passageMemoEmbedder{}, excerpt)
			assert.Equal(t, want, passageSelector(t, r.Context.Target))
			if mode == "principle" {
				assert.Equal(t, "Keep today's rule.", r.Context.Target.Body)
			}
		})
	}
}

func TestAsk_DensePassageShownBefore(t *testing.T) {
	db, _ := densePassageFixture(t)
	r, err := query.Ask(context.Background(), &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Score: 1}}}, graph.NewResolver(db), db, query.AskConfig{Query: "alpha", Embedder: &passageMemoEmbedder{}, Budget: 1500, MaxItems: 3, SearchLimit: 5, ExcerptTokens: 160, ShownBefore: map[string]bool{"target": true}})
	require.NoError(t, err)
	assert.Empty(t, r.Context.Target.Body)
	assert.Equal(t, "none", passageSelector(t, r.Context.Target))
}

func replaceDenseUnits(t *testing.T, db *index.DB, raw string, scores []float32) {
	t.Helper()
	_, err := db.Exec(`UPDATE notes SET body_raw=? WHERE id='target'`, raw)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM passage_vectors`)
	require.NoError(t, err)
	units := passage.Units(raw, "Passage title")
	require.Len(t, units, len(scores))
	for i, u := range units {
		x := scores[i]
		_, err = db.Exec(`INSERT INTO passage_vectors VALUES ('target', ?, ?, ?, ?)`, i, u.Start, u.End, index.EncodeEmbedding([]float32{x, 1 - x}))
		require.NoError(t, err)
	}
}

func TestAsk_DensePassageBudgetAndTies(t *testing.T) {
	for _, tc := range []struct {
		name         string
		words, cap   int
		scores       []float32
		want, absent string
	}{
		{"expanded_cap", 26, 160, []float32{0, 0.8, 1, 0, 0.9}, "Use a stable snapshot", "alpha"},
		{"drop_lowest", 42, 160, []float32{0, 0.8, 1, 0, 0.9}, "Preserve the immutable receipt", "Use a stable snapshot"},
		{"cut_one", 80, 20, []float32{0, 0.8, 1, 0, 0.9}, "Preserve", "Verify the restored state"},
		{"ties_earlier", 0, 160, []float32{0, 1, 1, 1, 1}, "An irrelevant distraction", "Verify the restored state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := densePassageFixture(t)
			padding := strings.Repeat("detail ", tc.words)
			raw := passageParagraph("alpha", 100) + "\n\n## Answer\n\nUse a stable snapshot " + padding + "now.\n\nPreserve the immutable receipt " + padding + "now.\n\nAn irrelevant distraction.\n\n## Final\n\nVerify the restored state " + padding + "now."
			replaceDenseUnits(t, db, raw, tc.scores)
			r := askDensePassage(t, db, &passageMemoEmbedder{}, tc.cap)
			assert.Equal(t, "dense", passageSelector(t, r.Context.Target))
			assert.Contains(t, r.Context.Target.Body, tc.want)
			assert.NotContains(t, r.Context.Target.Body, tc.absent)
			assert.LessOrEqual(t, memory.EstimateTokens(r.Context.Target.Body), tc.cap+tc.cap/2)
			assert.LessOrEqual(t, r.Context.UsedTokens, r.Context.BudgetTokens)
			if tc.name == "expanded_cap" {
				assert.Greater(t, memory.EstimateTokens(r.Context.Target.Body), tc.cap)
			}
		})
	}
}
