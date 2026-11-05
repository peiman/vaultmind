package query_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
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

// overflowAnswer is the passage the question asks for (~290 tokens). It sits
// in the second section of a context note larger than the whole ask budget.
var overflowAnswer = passageParagraph("register command checks", 140)

// overflowDB stores a small target related to a ~1,400-token "big" note (an
// opening paragraph, the answer under its own heading, then filler) and,
// ranked below it, linked to a ~300-token "small" note.
func overflowDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "overflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	target := "A short target note."
	links := []index.LinkRecord{
		{DstNoteID: "big", DstRaw: "big", EdgeType: "explicit_relation", Resolved: true},
		{DstNoteID: "small", DstRaw: "small", EdgeType: "link", Resolved: true},
	}
	require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "target", Path: "target.md", Type: "reference", Title: "Target", BodyRaw: target, BodyText: target, Links: links}))
	big := passageParagraph("intro", 74) + "\n\n## Adding a command\n\n" + overflowAnswer + "\n\n" + passageParagraph("unrelated", 400)
	require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "big", Path: "big.md", Type: "reference", Title: "Big guide", BodyRaw: big, BodyText: parser.StripForFTS(big)}))
	small := passageParagraph("smaller", 150)
	require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "small", Path: "small.md", Type: "reference", Title: "Small note", BodyRaw: small, BodyText: small}))
	return db
}

// askOverflow returns the ask result and its "big" context item, which must
// be the first item: it outranks "small".
func askOverflow(t *testing.T, db *index.DB, excerpt int) (*query.AskResult, memory.ContextItem) {
	t.Helper()
	retriever := &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Title: "Target", Score: 1}}}
	r, err := query.Ask(context.Background(), retriever, graph.NewResolver(db), db, query.AskConfig{Query: "register command checks", Budget: 600, MaxItems: 3, SearchLimit: 5, ExcerptTokens: excerpt})
	require.NoError(t, err)
	require.NotNil(t, r.Context)
	require.NotEmpty(t, r.Context.Context)
	require.Equal(t, "big", r.Context.Context[0].ID)
	return r, r.Context.Context[0]
}

func itemSelector(t *testing.T, item memory.ContextItem) string {
	t.Helper()
	encoded, err := json.Marshal(item)
	require.NoError(t, err)
	var fields map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &fields))
	selector, _ := fields["passage_selector"].(string)
	return selector
}

// #331: default ask (excerpt 0) packed a context note whole or not at all, so
// a note larger than the room left came back as its title. It now delivers
// the passage the question asks for, sized to the room left, marked as an
// excerpt with the selector that chose it. The room is the higher-ranked
// note's: the whole answer must arrive, not what a lower-ranked note left.
func TestAsk_OverflowingContextItemGetsPassageExcerpt(t *testing.T) {
	r, item := askOverflow(t, overflowDB(t), 0)

	assert.Contains(t, item.Body, overflowAnswer, "the over-budget note must deliver the whole matching passage, not nothing")
	assert.Contains(t, item.Body, "## Adding a command")
	assert.NotContains(t, item.Body, "intro padding", "the selector must replace the opening paragraph")
	assert.True(t, item.BodyIncluded && item.BodyExcerpted)
	assert.False(t, item.BodyOmittedBudget)
	assert.Equal(t, "lexical", itemSelector(t, item))
	assert.LessOrEqual(t, r.Context.UsedTokens, r.Context.BudgetTokens)

	var out bytes.Buffer
	require.NoError(t, query.FormatAsk(r, &out))
	assert.Contains(t, out.String(), "register command checks")
	assert.Contains(t, out.String(), "as excerpt")
}

// Text-mode pointers-only renders no bodies, so the overflow note keeps the
// packer's opening excerpt and skips the selection, as the target does.
func TestAsk_OverflowPointersOnlySkipsSelection(t *testing.T) {
	db := overflowDB(t)
	retriever := &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Title: "Target", Score: 1}}}
	r, err := query.Ask(context.Background(), retriever, graph.NewResolver(db), db, query.AskConfig{Query: "register command checks", Budget: 600, MaxItems: 3, SearchLimit: 5, PointersOnly: true})
	require.NoError(t, err)
	require.NotEmpty(t, r.Context.Context)
	item := r.Context.Context[0]

	assert.Equal(t, "none", itemSelector(t, item))
	assert.Contains(t, item.Body, "intro padding")
}

// The hook path (excerpt > 0) is #330's measured result. Context items there
// keep their existing excerpt and carry no selector.
func TestAsk_ExcerptPathLeavesContextItemsUnselected(t *testing.T) {
	_, item := askOverflow(t, overflowDB(t), 160)

	assert.Empty(t, itemSelector(t, item))
	assert.Contains(t, item.Body, "intro padding")
}
