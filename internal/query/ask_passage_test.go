package query_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

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
		{"warning_heading", passageParagraph("intro", 74) + "\n\n## Unsafe production configuration (do not use)\n\n- Disable certificate validation with TLS_VERIFY=false.", "TLS_VERIFY", "## Unsafe production configuration (do not use)"},
		{"late_keyphrase", passageParagraph("intro", 74) + "\n\n" + strings.Repeat("background ", 120) + "Set WAL_RECOVERY=immutable to recover the database.", "WAL_RECOVERY", "WAL_RECOVERY=immutable"},
		{"unspaced_cjk", passageParagraph("intro", 74) + "\n\n" + strings.Repeat("背景", 180) + "検索は重要である。", "検索", "検索は重要である。"},
		{"setext_warning", passageParagraph("intro", 74) + "\n\nUnsafe (do not use)\n-------------------\n\nSet TLS_VERIFY=false.", "TLS_VERIFY", "Unsafe (do not use)\n-------------------"},
		{"nested_warning", passageParagraph("intro", 74) + "\n\n## Deprecated (do not use)\n\n### Configuration\n\nSet TLS_VERIFY=false.", "TLS_VERIFY", "## Deprecated (do not use)\n### Configuration"},
		{"heading_consumes_cap", passageParagraph("intro", 74) + "\n\n## Unsafe production configuration (do not use)\n\nSet TLS_VERIFY=false. " + strings.Repeat("padding ", 90), "TLS_VERIFY", "## Unsafe production configuration (do not use)"},
		{"oversized_heading", passageParagraph("intro", 74) + "\n\n## Unsafe " + strings.Repeat("warning ", 100) + "\n\nSet TLS_VERIFY=false.", "TLS_VERIFY", passageParagraph("intro", 74)},
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
			assert.True(t, utf8.ValidString(result.Context.Target.Body))
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
	return passageDBWithTitle(t, body, "Passage title")
}

func passageDBWithTitle(t *testing.T, body, title string) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "passage.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "target", Path: "target.md", Type: "arc", Title: title, BodyRaw: body, BodyText: parser.StripForFTS(body), Links: []index.LinkRecord{{DstNoteID: "neighbor", DstRaw: "neighbor", EdgeType: "link", Resolved: true}}}))
	neighbor := passageParagraph("intro", 74) + "\n\n" + passageParagraph("alpha beta gamma", 72)
	require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "neighbor", Path: "neighbor.md", Type: "reference", Title: "Neighbor", BodyRaw: neighbor, BodyText: parser.StripForFTS(neighbor)}))
	return db
}

// A selected later passage carries its governing headings in document order.
// The note title is already on the header, so an H1 that is that title — or
// the same title written as a setext heading, with closing hashes, or with
// different case and spacing — is left out. A different H1 stays.
func TestAsk_PassageHeadingPath(t *testing.T) {
	intro := passageParagraph("intro", 74)
	later := "Set TLS_VERIFY=false."
	warning := "## Unsafe production configuration (do not use)\n\n- Disable certificate validation with TLS_VERIFY=false."
	tests := []struct {
		name, title, body, prefix string
		absent                    []string
	}{
		{
			name: "nested_top_down_omits_title", title: "Title",
			body:   intro + "\n\n# Title\n\n## Section\n\n### Sub\n\n" + later,
			prefix: "## Section\n### Sub\n\n" + later, absent: []string{"# Title"},
		},
		{
			name: "section_only", title: "Title",
			body: intro + "\n\n## Section\n\n" + later, prefix: "## Section\n\n" + later,
		},
		{
			name: "different_h1_kept", title: "Title",
			body: intro + "\n\n# Other\n\n## Section\n\n" + later, prefix: "# Other\n## Section\n\n" + later,
		},
		{
			name: "setext_h1_is_the_title", title: "Title",
			body:   intro + "\n\nTitle\n=====\n\n## Section\n\n" + later,
			prefix: "## Section\n\n" + later, absent: []string{"====="},
		},
		{
			name: "h1_effectively_the_title", title: "The count that lied",
			body:   intro + "\n\n# the   count that lied #\n\n## What I did not do\n\n" + later,
			prefix: "## What I did not do\n\n" + later, absent: []string{"count that lied"},
		},
		{
			name: "warning_heading", title: "Passage title",
			body: intro + "\n\n" + warning, prefix: warning,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := passageDBWithTitle(t, tt.body, tt.title)
			retriever := &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Title: tt.title, Score: 1}}}
			result, err := query.Ask(context.Background(), retriever, graph.NewResolver(db), db, query.AskConfig{Query: "TLS_VERIFY", Budget: 1500, MaxItems: 3, SearchLimit: 5, ExcerptTokens: 160})
			require.NoError(t, err)
			require.NotNil(t, result.Context)
			require.NotNil(t, result.Context.Target)
			got := result.Context.Target.Body
			assert.True(t, strings.HasPrefix(got, tt.prefix), "got %q", got)
			for _, bad := range tt.absent {
				assert.NotContains(t, got, bad)
			}
			assert.True(t, utf8.ValidString(got))
			assert.LessOrEqual(t, memory.EstimateTokens(got), 160)
			var out bytes.Buffer
			require.NoError(t, query.FormatAsk(result, &out))
			assert.Contains(t, out.String(), "["+result.Context.Target.Frontmatter["type"].(string)+"] "+tt.title)
		})
	}
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

func TestAsk_PassagePointersEqualMain(t *testing.T) {
	body := passageParagraph("intro", 74) + "\n\n## Unsafe production configuration (do not use)\n\n- Disable certificate validation with TLS_VERIFY=false."
	db := passageDB(t, body)
	resolver := graph.NewResolver(db)
	baseline, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "target", Budget: 1500, Depth: 1, MaxItems: 3, Slim: true, ExcerptTokens: 160})
	require.NoError(t, err)
	retriever := &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Score: 1}}}
	for _, jsonOutput := range []bool{false, true} {
		result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{Query: "TLS_VERIFY", Budget: 1500, MaxItems: 3, SearchLimit: 5, ExcerptTokens: 160, PointersOnly: true, JSONOutput: jsonOutput})
		require.NoError(t, err)
		if jsonOutput {
			assert.Contains(t, result.Context.Target.Body, "TLS_VERIFY=false")
			continue
		}
		assert.Equal(t, baseline, result.Context)
		var actual, expected bytes.Buffer
		require.NoError(t, query.FormatAskPointersOnly(result, &actual))
		main := *result
		main.Context = baseline
		require.NoError(t, query.FormatAskPointersOnly(&main, &expected))
		assert.Equal(t, expected.String(), actual.String(), "includes assembled-and-withheld token count")
	}
}

func TestAsk_PassageFirstBlockUnchanged(t *testing.T) {
	first := "Opening context. " + strings.Repeat("background ", 70) + " Set WAL_RECOVERY=immutable to recover the database."
	db := passageDB(t, first+"\n\nUnrelated ending.")
	resolver := graph.NewResolver(db)
	baseline, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "target", Budget: 1500, Depth: 1, MaxItems: 3, Slim: true, ExcerptTokens: 160})
	require.NoError(t, err)
	retriever := &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Score: 1}}}
	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{Query: "WAL_RECOVERY", Budget: 1500, MaxItems: 3, SearchLimit: 5, ExcerptTokens: 160})
	require.NoError(t, err)
	assert.Equal(t, baseline, result.Context)
}

// The verifier's first-block fixture: the qualification must travel with the
// retained prose, even when lexical selection keeps the first block.
func TestAsk_PassageFirstBlockHeading(t *testing.T) {
	warning := "## Unsafe production configuration (do not use)"
	first := "Set TLS_VERIFY=false. " + strings.Repeat("padding ", 90) + "ends."
	tests := []struct {
		name, lead, question, want string
		cap                        int
		unchanged                  bool
	}{
		{"warning", warning + "\n\n", "TLS_VERIFY", warning + "\n\nSet TLS_VERIFY=false.", 160, false},
		{"no_query_overlap", warning + "\n\n", "absent", warning + "\n\nSet TLS_VERIFY=false.", 160, false},
		{"nested", "# Passage title\n\n" + warning + "\n\n### TLS settings\n\n", "TLS_VERIFY", "### TLS settings\n\nSet TLS_VERIFY=false.", 160, false},
		{"long_nearest", "", "TLS_VERIFY", "#### Do not use this unsafe production setting locally extra…\n\nSet TLS_VERIFY=false.", 160, false},
		{"setext", "Do not use in production systems now\n------------------------------------\n\n", "TLS_VERIFY", "Do not use in production systems now…\n\nSet TLS_VERIFY=false.", 160, false},
		{"setext_short", "Unsafe (do not use)\n-------------------\n\n", "TLS_VERIFY", "Unsafe (do not use)\n-------------------\n\nSet TLS_VERIFY=false.", 160, false},
		{"title_only", "# Passage title\n\n", "TLS_VERIFY", "Set TLS_VERIFY=false.", 160, true},
		{"setext_title_only", "Passage title\n=============\n\n", "TLS_VERIFY", "Set TLS_VERIFY=false.", 160, true},
		{"cap", warning + "\n\n", "TLS_VERIFY", warning + "\n\nSet TLS_VERIFY=false.", 19, false},
		{"long_first_block_cap", warning + "\n\n", "TLS_VERIFY", warning + "\n\nSet TLS_VERIFY=false", 160, false},
		{"no_prose_room", warning + "\n\n", "TLS_VERIFY", warning, 13, false},
		{"prose_does_not_fit", warning + "\n\n", "TLS_VERIFY", warning, 14, false},
		{"heading_exceeds_limit", warning + "\n\n", "TLS_VERIFY", "Set TLS_VERIFY=false.", 11, true},
		{"wide_token", "", "TLS_VERIFY", strings.Repeat("A", 61) + "…\n\nSet TLS_VERIFY=false.", 160, false},
		{"oversized_heading", "## Unsafe " + strings.Repeat("warning ", 100) + "\n\n", "TLS_VERIFY", "## Unsafe warning warning warning warning warning warning…\n\nSet TLS_VERIFY=false.", 160, false},
		{"principle", warning + "\n\n", "TLS_VERIFY", "Keep the decision rule.", 160, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.lead + first + "\n\nUnrelated ending."
			if tt.name == "long_first_block_cap" {
				body = tt.lead + strings.ReplaceAll(first, "false.", "false") + "\n\nUnrelated ending."
			}
			if tt.name == "long_nearest" {
				body = "# Passage title\n\n" +
					"## Ancestor qualification that must not travel with the block\n\n" +
					"### Middle heading that is also not the nearest\n\n" +
					"#### Do not use this unsafe production setting locally extra words that run past the cap\n\n" +
					first + "\n\nUnrelated ending."
			}
			if tt.name == "wide_token" {
				// One token wider than 16, so the cap cuts by rune. The underline
				// stays short enough that the heading is not itself treated as prose.
				body = strings.Repeat("A", 65) + "\n" + strings.Repeat("-", 14) + "\n\n" + first + "\n\nUnrelated ending."
			}
			if tt.name == "principle" {
				body += "\n\n## Principle\n\nKeep the decision rule."
			}
			db := passageDB(t, body)
			resolver := graph.NewResolver(db)
			baseline, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "target", Budget: 1500, Depth: 1, MaxItems: 3, Slim: true, ExcerptTokens: tt.cap})
			require.NoError(t, err)
			retriever := &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Title: "Passage title", Score: 1}}}
			result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{Query: tt.question, Budget: 1500, MaxItems: 3, SearchLimit: 5, ExcerptTokens: tt.cap})
			require.NoError(t, err)
			got := result.Context.Target.Body
			if tt.want == "" {
				assert.Empty(t, got)
			} else {
				assert.Contains(t, got, tt.want)
			}
			assert.NotContains(t, got, "Passage title", "title is already delivered separately")
			if tt.name != "warning" && tt.name != "no_query_overlap" && tt.name != "cap" && tt.name != "long_first_block_cap" && tt.name != "no_prose_room" && tt.name != "prose_does_not_fit" {
				assert.NotContains(t, got, warning)
			}
			assert.LessOrEqual(t, strings.Count(got, warning), 1)
			if tt.name == "nested" {
				assert.NotContains(t, got, "Unsafe production configuration")
			}
			if tt.name == "long_nearest" || tt.name == "oversized_heading" || tt.name == "setext" || tt.name == "wide_token" {
				head, _, _ := strings.Cut(got, "\n\n")
				assert.LessOrEqual(t, memory.EstimateTokens(head), 16, "nearest heading is capped at 16 tokens")
				assert.NotContains(t, got, "Ancestor qualification")
				assert.NotContains(t, got, "Middle heading")
			}
			if tt.name == "no_prose_room" || tt.name == "prose_does_not_fit" {
				assert.NotContains(t, got, "TLS_VERIFY", "the block is trimmed before the heading is")
			}
			assert.True(t, utf8.ValidString(got))
			assert.LessOrEqual(t, memory.EstimateTokens(got), tt.cap)
			assert.LessOrEqual(t, result.Context.UsedTokens, result.Context.BudgetTokens)
			assert.Equal(t, baseline.Context, result.Context.Context)
			assert.Equal(t, baseline.UsedTokens-memory.EstimateTokens(baseline.Target.Body)+memory.EstimateTokens(got), result.Context.UsedTokens)
			if tt.unchanged {
				assert.Equal(t, baseline, result.Context)
			}
			var out bytes.Buffer
			require.NoError(t, query.FormatAsk(result, &out))
			if tt.want != "" {
				assert.Contains(t, out.String(), strings.ReplaceAll(tt.want, "\n", "\n    "))
			}
		})
	}
}
