package query_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Equal token lengths and corpus frequencies make two distinct query terms
// score exactly twice one. Each paragraph fits 160 tokens, but pairs do not.
func passageParagraph(terms string, count int) string {
	return terms + " " + strings.Repeat("padding ", count) + "ends."
}

func TestAsk_PassageDelivery(t *testing.T) {
	first := passageParagraph("alpha", 74)
	tests := []struct{ name, body, question, want string }{
		{"later_above_margin", first + "\n\n" + passageParagraph("alpha beta gamma", 72), "alpha beta gamma", "alpha beta gamma"},
		{"later_just_below_margin", first + "\n\n" + passageParagraph("beta gamma", 74), "alpha beta gamma", first},
		{"later_below_margin", first + "\n\n" + passageParagraph("alpha alpha", 73), "alpha", first},
		{"exact_two_margin", first + "\n\n" + passageParagraph("beta gamma", 73), "alpha beta gamma", "beta gamma"},
		{"zero_first", passageParagraph("intro", 74) + "\n\n" + passageParagraph("beta gamma", 73), "beta gamma", "beta gamma"},
		{"principle", "## Trigger\n\n" + first + "\n\n## Principle\n\nKeep the decision rule.\n\n## Source\n\n" + passageParagraph("beta gamma", 73), "beta gamma", "Keep the decision rule."},
		{"raw_identifier", passageParagraph("intro", 74) + "\n\nSet `text_factory` to bytes for raw results. " + strings.Repeat("padding ", 65), "text_factory", "`text_factory`"},
		{"unicode_keyword_normalization", passageParagraph("intro", 74) + "\n\n" + passageParagraph("CAFÉ NODE_TIMEOUT", 72), "cafe node_timeout", "CAFÉ NODE_TIMEOUT"},
		{"long_sentence_cap", passageParagraph("intro", 74) + "\n\nThe beta answer " + strings.Repeat("padding ", 150) + "ends.", "beta", "The beta answer"},
		{"no_overlap", first + "\n\n" + passageParagraph("beta", 74), "absent", first},
		{"headings_and_fences", "# Overview\n\n" + passageParagraph("intro", 74) + "\n\n```python\nbeta gamma code.\n```\n\n# Answer\n\n" + passageParagraph("beta gamma", 73), "beta gamma", "beta gamma padding"},
		{"fenced_body_fallback", "```python\nbeta gamma code.\n```", "beta gamma", "```python\nbeta gamma code.\n```"},
		{"punctuation_fallback", "!!!\n\n???", "beta gamma", "!!!"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := passageDB(t, tt.body)
			// Supply a fixed search result so these tests guard delivery, independently
			// of ranking. The real SQLite index supplies normalization and corpus IDF.
			retriever := &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Title: "Passage title", Score: 1}}}
			result, err := query.Ask(context.Background(), retriever, graph.NewResolver(db), db, query.AskConfig{Query: tt.question, Budget: 1500, MaxItems: 3, SearchLimit: 5, ExcerptTokens: 160})
			require.NoError(t, err)
			require.NotNil(t, result.Context)
			require.NotNil(t, result.Context.Target)
			assert.Contains(t, result.Context.Target.Body, tt.want)
			assert.LessOrEqual(t, memory.EstimateTokens(result.Context.Target.Body), 160)
			assert.LessOrEqual(t, result.Context.UsedTokens, result.Context.BudgetTokens)
			var out bytes.Buffer
			require.NoError(t, query.FormatAsk(result, &out))
			assert.Contains(t, out.String(), strings.ReplaceAll(tt.want, "\n", "\n    "))
			assert.Contains(t, out.String(), "[arc] Passage title")
		})
	}
}

func passageDB(t *testing.T, body string) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "passage.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "target", Path: "target.md", Type: "arc", Title: "Passage title", BodyRaw: body, BodyText: parser.StripForFTS(body), Links: []index.LinkRecord{{DstNoteID: "neighbor", DstRaw: "neighbor", EdgeType: "link", Resolved: true}}}))
	neighbor := passageParagraph("intro", 74) + "\n\n" + passageParagraph("alpha beta gamma", 72)
	require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "neighbor", Path: "neighbor.md", Type: "reference", Title: "Neighbor", BodyRaw: neighbor, BodyText: parser.StripForFTS(neighbor)}))
	return db
}

func TestAsk_PassageTargetOnly(t *testing.T) {
	body := passageParagraph("intro", 74) + "\n\n" + passageParagraph("beta gamma", 73)
	db := passageDB(t, body)
	resolver := graph.NewResolver(db)
	baseline, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "target", Budget: 1500, Depth: 1, MaxItems: 3, Slim: true, ExcerptTokens: 160})
	require.NoError(t, err)
	require.NotEmpty(t, baseline.Context)
	retriever := &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Score: 1}}}
	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{Query: "beta gamma", Budget: 1500, MaxItems: 3, SearchLimit: 5, ExcerptTokens: 160})
	require.NoError(t, err)
	assert.Contains(t, result.Context.Target.Body, "beta gamma")
	assert.Equal(t, baseline.Context, result.Context.Context, "context excerpts must remain byte-identical")
	assert.Equal(t, baseline.Target.Frontmatter, result.Context.Target.Frontmatter)
	assert.Equal(t, baseline.UsedTokens-memory.EstimateTokens(baseline.Target.Body)+memory.EstimateTokens(result.Context.Target.Body), result.Context.UsedTokens)
	// The shared memory-pack API and excerpt-off ask keep their prior bodies.
	full, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{Query: "beta gamma", Budget: 4000, MaxItems: 3, SearchLimit: 5})
	require.NoError(t, err)
	assert.Equal(t, body, full.Context.Target.Body)
}

// A larger target window must use only the space left after the existing
// context reservation, even when the per-item cap would allow more text.
func TestAsk_PassageRespectsRemainingBudget(t *testing.T) {
	db := passageDB(t, "Intro.\n\n"+passageParagraph("beta gamma", 73))
	resolver := graph.NewResolver(db)
	const budget = 240
	baseline, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "target", Budget: budget, Depth: 1, MaxItems: 3, Slim: true, ExcerptTokens: 160})
	require.NoError(t, err)
	require.NotEmpty(t, baseline.Context)
	available := budget - baseline.UsedTokens + memory.EstimateTokens(baseline.Target.Body)
	require.Positive(t, available)
	require.Less(t, available, 160, "fixture must constrain the excerpt below its usual cap")
	retriever := &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Score: 1}}}
	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{Query: "beta gamma", Budget: budget, MaxItems: 3, SearchLimit: 5, ExcerptTokens: 160})
	require.NoError(t, err)
	assert.Contains(t, result.Context.Target.Body, "beta gamma")
	assert.LessOrEqual(t, memory.EstimateTokens(result.Context.Target.Body), available)
	assert.LessOrEqual(t, result.Context.UsedTokens, budget)
	assert.Equal(t, baseline.Context, result.Context.Context)
}

// Passage selection is optional enrichment: an unavailable derived FTS index
// must retain the already-packed excerpt, rather than discard the answer or
// return a partial replacement with incorrect token accounting.
func TestAsk_PassageFailureKeepsPackedExcerpt(t *testing.T) {
	db := passageDB(t, passageParagraph("intro", 74)+"\n\n"+passageParagraph("beta gamma", 73))
	resolver := graph.NewResolver(db)
	baseline, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "target", Budget: 1500, Depth: 1, MaxItems: 3, Slim: true, ExcerptTokens: 160})
	require.NoError(t, err)
	_, err = db.Exec("DROP TABLE fts_notes")
	require.NoError(t, err)
	retriever := &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Score: 1}}}
	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{Query: "beta gamma", Budget: 1500, MaxItems: 3, SearchLimit: 5, ExcerptTokens: 160})
	require.NoError(t, err)
	assert.Equal(t, baseline, result.Context)
	var out bytes.Buffer
	require.NoError(t, query.FormatAsk(result, &out))
	assert.Contains(t, out.String(), baseline.Target.Body)
}
