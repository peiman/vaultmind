package query_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
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

var denseQuestion = "alpha " + strings.Repeat(".", 40)

func askDensePassage(t *testing.T, db *index.DB, emb *passageMemoEmbedder, excerpt int) *query.AskResult {
	t.Helper()
	return askDensePassageQuery(t, db, emb, denseQuestion, excerpt, 1500)
}

// densePart requires the body to open with the lexical pick, byte for byte as
// a run without an embedder delivers it, and returns what dense added after.
func densePart(t *testing.T, db *index.DB, r *query.AskResult, excerpt int) string {
	t.Helper()
	lex := askDensePassageQuery(t, db, nil, denseQuestion, excerpt, 1500).Context.Target.Body
	require.True(t, strings.HasPrefix(r.Context.Target.Body, lex+"\n…\n"), r.Context.Target.Body)
	return strings.TrimPrefix(r.Context.Target.Body, lex+"\n…\n")
}

func askDensePassageQuery(t *testing.T, db *index.DB, emb embedding.Embedder, question string, excerpt, budget int) *query.AskResult {
	t.Helper()
	r, err := query.Ask(context.Background(), &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Title: "Passage title", Score: 1}}}, graph.NewResolver(db), db, query.AskConfig{Query: question, Embedder: emb, Budget: budget, MaxItems: 3, SearchLimit: 5, ExcerptTokens: excerpt})
	require.NoError(t, err)
	require.NotNil(t, r.Context)
	require.NotNil(t, r.Context.Target)
	return r
}

// The lexical pick answers exact-term questions; dense ranking answers
// paraphrases. Each block is its own paragraph, so the lexical window is the
// History section alone, and the scores decide where dense ranks it.
const (
	unionLex   = "## History\n\nThis directive appeared in version 1.11.8."
	unionDense = "## Usage\n\nEnable the listener for incoming peers.\n\nEach peer negotiates a session key.\n\nRestart after changing the listener."
	unionQuery = "directive appeared version"
)

func unionPassageFixture(t *testing.T, scores []float32) *index.DB {
	t.Helper()
	db, _ := densePassageFixture(t)
	// The opening paragraph is the packed excerpt; keeping it shorter than the
	// lexical pick means swapping one for the other frees no budget.
	replaceDenseUnits(t, db, passageParagraph("intro", 4)+"\n\n"+unionDense+"\n\n"+unionLex, scores)
	return db
}

// Dense alone ranks the three Usage blocks above History and would drop the
// version line; the union keeps it and still adds the dense blocks.
func TestAsk_DensePassageUnionKeepsExactTerm(t *testing.T) {
	db := unionPassageFixture(t, []float32{0, 1, 0.9, 0.8, 0.1})
	r := askDensePassageQuery(t, db, &passageMemoEmbedder{}, unionQuery, 160, 1500)
	assert.Equal(t, "dense", passageSelector(t, r.Context.Target))
	assert.Equal(t, unionLex+"\n…\n"+unionDense, r.Context.Target.Body)
}

// Dense ranks the lexical pick first; it is delivered once, and the two
// other top-three blocks join it.
func TestAsk_DensePassageUnionDedupe(t *testing.T) {
	db := unionPassageFixture(t, []float32{0, 0.9, 0.8, 0.7, 1})
	r := askDensePassageQuery(t, db, &passageMemoEmbedder{}, unionQuery, 160, 1500)
	assert.Equal(t, "dense", passageSelector(t, r.Context.Target))
	assert.Equal(t, 1, strings.Count(r.Context.Target.Body, "This directive appeared"))
	assert.Equal(t, unionLex+"\n…\n## Usage\n\nEnable the listener for incoming peers.\n\nEach peer negotiates a session key.", r.Context.Target.Body)
}

// Only the top three blocks are candidates. History ranks second and is
// already the lexical pick; its slot stays empty rather than going to the
// fourth block, which barely matches.
func TestAsk_DensePassageNoBackfill(t *testing.T) {
	db := unionPassageFixture(t, []float32{0, 1, 0.1, 0.9, 0.95})
	r := askDensePassageQuery(t, db, &passageMemoEmbedder{}, unionQuery, 160, 1500)
	assert.Equal(t, "dense", passageSelector(t, r.Context.Target))
	assert.NotContains(t, r.Context.Target.Body, "Each peer negotiates")
	assert.Equal(t, unionLex+"\n…\n## Usage\n\nEnable the listener for incoming peers.\n…\nRestart after changing the listener.", r.Context.Target.Body)
}

// Two tokens of room past the lexical pick fit no dense block: the pick
// survives whole and the dense blocks are dropped.
func TestAsk_DensePassageUnionBudget(t *testing.T) {
	db := unionPassageFixture(t, []float32{0, 1, 0.9, 0.8, 0.1})
	lexOnly := askDensePassageQuery(t, db, nil, unionQuery, 160, 1500)
	require.Equal(t, unionLex, lexOnly.Context.Target.Body)
	budget := lexOnly.Context.UsedTokens + 2
	r := askDensePassageQuery(t, db, &passageMemoEmbedder{}, unionQuery, 160, budget)
	assert.Equal(t, "lexical", passageSelector(t, r.Context.Target))
	assert.Equal(t, unionLex, r.Context.Target.Body)
	assert.LessOrEqual(t, r.Context.UsedTokens, budget)
	body := memory.EstimateTokens(r.Context.Target.Body)
	assert.LessOrEqual(t, body, min(2*160, budget-r.Context.UsedTokens+body))
}

// Without vectors the delivered body is the lexical pick, byte for byte.
func TestAsk_DensePassageNoVectorsIsLexical(t *testing.T) {
	db := unionPassageFixture(t, []float32{0, 1, 0.9, 0.8, 0.1})
	_, err := db.Exec(`DELETE FROM passage_vectors`)
	require.NoError(t, err)
	r := askDensePassageQuery(t, db, &passageMemoEmbedder{}, unionQuery, 160, 1500)
	assert.Equal(t, "lexical", passageSelector(t, r.Context.Target))
	assert.Equal(t, unionLex, r.Context.Target.Body)
}

func TestAsk_DensePassageTopThree(t *testing.T) {
	db, _ := densePassageFixture(t)
	emb := &passageMemoEmbedder{}
	r := askDensePassage(t, db, emb, 160)
	// The opening paragraph ranks third (it ties the distraction at zero and
	// comes first) but no longer fits after the lexical pick; its slot stays
	// empty, so the distraction is not delivered.
	assert.Equal(t, "## Answer\n\nUse a stable snapshot. Preserve the immutable receipt.\n…\n## Final\n\nVerify the restored state.", densePart(t, db, r, 160))
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
		{"expanded_cap", 10, 160, []float32{0, 0.8, 1, 0, 0.9}, "Use a stable snapshot", "An irrelevant distraction"},
		{"skip_unfit", 26, 160, []float32{0, 0.8, 1, 0, 0.9}, "Preserve the immutable receipt", "Use a stable snapshot"},
		{"oversized_skipped", 80, 20, []float32{0, 0.8, 1, 0.85, 0.9}, "An irrelevant distraction", "Preserve"},
		{"ties_earlier", 0, 160, []float32{0, 1, 1, 1, 1}, "An irrelevant distraction", "Verify the restored state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := densePassageFixture(t)
			padding := strings.Repeat("detail ", tc.words)
			raw := passageParagraph("alpha", 100) + "\n\n## Answer\n\nUse a stable snapshot " + padding + "now.\n\nPreserve the immutable receipt " + padding + "now.\n\nAn irrelevant distraction.\n\n## Final\n\nVerify the restored state " + padding + "now."
			replaceDenseUnits(t, db, raw, tc.scores)
			r := askDensePassage(t, db, &passageMemoEmbedder{}, tc.cap)
			assert.Equal(t, "dense", passageSelector(t, r.Context.Target))
			dense := densePart(t, db, r, tc.cap)
			assert.Contains(t, dense, tc.want)
			assert.NotContains(t, dense, tc.absent)
			assert.LessOrEqual(t, memory.EstimateTokens(r.Context.Target.Body), 2*tc.cap)
			assert.LessOrEqual(t, r.Context.UsedTokens, r.Context.BudgetTokens)
			if tc.name == "expanded_cap" {
				assert.Greater(t, memory.EstimateTokens(r.Context.Target.Body), tc.cap)
			}
			if tc.name == "skip_unfit" {
				assert.Contains(t, dense, "Verify the restored state")
				assert.NotContains(t, dense, "An irrelevant distraction", "an unfit top-three block is not replaced by a lower-ranked one")
			}
			if tc.name == "oversized_skipped" {
				assert.NotContains(t, dense, "Verify the restored state", "oversized blocks ranked above the distraction are skipped; the distraction still joins")
			}
		})
	}
}

func TestAsk_DensePassageWholeParagraph(t *testing.T) {
	db, _ := densePassageFixture(t)
	paragraph := "What I changed: the regression guard asserts the query plan, not elapsed time. The answer is the covering index on receipt_id. Keep this qualification with the answer."
	raw := passageParagraph("alpha", 100) + "\n\n## Guard\n\n" + paragraph + "\n\nAnother useful result.\n\nA final useful result."
	replaceDenseUnits(t, db, raw, []float32{0, 1, 0.05, 0.01, 0.95, 0.9})
	r := askDensePassage(t, db, &passageMemoEmbedder{}, 160)
	assert.Equal(t, "dense", passageSelector(t, r.Context.Target))
	assert.Equal(t, "## Guard\n\n"+paragraph+"\n\nAnother useful result.\n\nA final useful result.", densePart(t, db, r, 160))
}

func TestAsk_DensePassageBlockHeadings(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		scores           []float32
	}{
		{
			name:   "adjacent_with_whitespace_blank_line",
			body:   "## Answer\n\nFirst result.\n \t\nSecond result.\n\n## Final\n\nLast result.",
			want:   "## Answer\n\nFirst result.\n\nSecond result.\n…\n## Final\n\nLast result.",
			scores: []float32{1, 0.9, 0.8},
		},
		{
			name:   "skipped_block_same_heading",
			body:   "## Answer\n\nFirst result.\n\nOmitted result.\n\nSecond result.\n\n## Final\n\nLast result.",
			want:   "## Answer\n\nFirst result.\n…\nSecond result.\n…\n## Final\n\nLast result.",
			scores: []float32{0.9, 0.1, 0.8, 1},
		},
		{
			name:   "heading_returns_after_change",
			body:   "## Answer\n\nFirst result.\n## Other\nSecond result.\n## Answer\nLast result.",
			want:   "## Answer\n\nFirst result.\n…\n## Other\n\nSecond result.\n…\n## Answer\n\nLast result.",
			scores: []float32{1, 0.9, 0.8},
		},
		{
			name:   "fence_breaks_block_without_repeating_heading",
			body:   "## Answer\nFirst result.\n```\nnot delivered\n```\nSecond result.",
			want:   "## Answer\n\nFirst result.\n…\nSecond result.",
			scores: []float32{1, 0.9},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := densePassageFixture(t)
			raw := passageParagraph("alpha", 140) + "\n\n" + tc.body
			replaceDenseUnits(t, db, raw, append([]float32{0}, tc.scores...))
			r := askDensePassage(t, db, &passageMemoEmbedder{}, 160)
			assert.Equal(t, "dense", passageSelector(t, r.Context.Target))
			assert.Equal(t, tc.want, densePart(t, db, r, 160))
		})
	}
}

// The strongest sentence is deliberately not first: using the first score,
// a mean, or a sum instead of the maximum changes which three blocks win.
func TestAsk_DensePassageBlockMax(t *testing.T) {
	db, _ := densePassageFixture(t)
	paragraph := "A weak lead. The strongest sentence answers the question."
	raw := passageParagraph("alpha", 140) + "\n\n" + paragraph + "\n\nMedium one. Medium two. Medium three.\n\nRunner up.\n\nThird place."
	replaceDenseUnits(t, db, raw, []float32{0, 0.01, 1, 0.7, 0.7, 0.7, 0.9, 0.8})
	r := askDensePassage(t, db, &passageMemoEmbedder{}, 160)
	assert.Equal(t, "dense", passageSelector(t, r.Context.Target))
	assert.Equal(t, paragraph+"\n…\nRunner up.\n\nThird place.", densePart(t, db, r, 160))
}

// A block wider than the room left after the lexical pick is skipped, not
// cut down: when no block fits, the body is the lexical pick alone.
func TestAsk_DensePassageOversizedBlockNotTrimmed(t *testing.T) {
	db, _ := densePassageFixture(t)
	raw := passageParagraph("alpha", 100) + "\n\n## Answer\n\nDiscard this weak lead. Best answer " + strings.Repeat("detail ", 100) + "ends. Tail qualification."
	replaceDenseUnits(t, db, raw, []float32{0, 0.1, 1, 0.2})
	r := askDensePassage(t, db, &passageMemoEmbedder{}, 30)
	assert.Equal(t, "lexical", passageSelector(t, r.Context.Target))
	assert.Equal(t, askDensePassageQuery(t, db, nil, denseQuestion, 30, 1500).Context.Target.Body, r.Context.Target.Body)
	assert.NotContains(t, r.Context.Target.Body, "Best answer")
}
