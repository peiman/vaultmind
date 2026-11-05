package query_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/passage"
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

// tocGuide is a note larger than the ask budget. Its contents list repeats
// the question's words in short lines; the answer is a later section with
// prose, a code fence and a second step.
const tocGuide = "# Guide\n\nThis guide explains the project.\n\n## Contents\n\n" +
	"- [Overview](#overview)\n- [Register a command](#register-a-command)\n" +
	"- [Register a command flag](#register-a-command-flag)\n- [Release notes](#release-notes)\n\n" +
	"## Overview\n\nThe project is a scaffold for tools.\n\n" +
	"## Register a command\n\n### Step 1: Scaffold\n\nTo register a command, run the scaffold.\n\n" +
	"```bash\ntask scaffold name=demo\n```\n\n### Step 2: Declare options\n\nDeclare every option before wiring.\n\n" +
	"## Release notes\n\n"

// tocGuideAnswer is what the answer section must deliver: its second step
// and the code in its first.
var tocGuideAnswer = []string{"## Register a command", "task scaffold name=demo", "### Step 2: Declare options", "Declare every option before wiring."}

func tocGuideDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "toc.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	target := "A short target note."
	require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "target", Path: "target.md", Type: "reference", Title: "Target", BodyRaw: target, BodyText: target, Links: []index.LinkRecord{{DstNoteID: "guide", DstRaw: "guide", EdgeType: "explicit_relation", Resolved: true}}}))
	guide := tocGuide + passageParagraph("unrelated", 400)
	require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "guide", Path: "guide.md", Type: "reference", Title: "Guide", BodyRaw: guide, BodyText: parser.StripForFTS(guide)}))
	return db
}

func askGuide(t *testing.T, db *index.DB, emb embedding.Embedder, question string) memory.ContextItem {
	t.Helper()
	retriever := &stubRetriever{results: []retrieval.ScoredResult{{ID: "target", Title: "Target", Score: 1}}}
	r, err := query.Ask(context.Background(), retriever, graph.NewResolver(db), db, query.AskConfig{Query: question, Embedder: emb, Budget: 600, MaxItems: 3, SearchLimit: 5})
	require.NoError(t, err)
	require.NotEmpty(t, r.Context.Context)
	require.LessOrEqual(t, r.Context.UsedTokens, r.Context.BudgetTokens)
	item := r.Context.Context[0]
	require.Equal(t, "guide", item.ID)
	return item
}

// #331 v2: an over-budget note whose contents list repeats the question used
// to deliver that list, a line of links. Navigation is not content: the pick
// lands in the answer section, which is delivered whole, code included.
func TestAsk_OverflowSkipsContentsAndDeliversTheSection(t *testing.T) {
	item := askGuide(t, tocGuideDB(t), nil, "register command")

	for _, want := range tocGuideAnswer {
		assert.Contains(t, item.Body, want)
	}
	assert.NotContains(t, item.Body, "](#", "a contents list is navigation, not an answer")
	assert.Equal(t, "lexical", itemSelector(t, item))
}

// The same holds when the dense lane ranks the contents list first: it is
// skipped, and the next block grows to its section.
func TestAsk_OverflowDenseSkipsContents(t *testing.T) {
	db := tocGuideDB(t)
	var raw string
	require.NoError(t, db.QueryRow(`SELECT body_raw FROM notes WHERE id='guide'`).Scan(&raw))
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS passage_vectors (note_id TEXT NOT NULL REFERENCES notes(id), unit_idx INTEGER NOT NULL, start INTEGER NOT NULL, end INTEGER NOT NULL, dense BLOB NOT NULL, PRIMARY KEY(note_id,unit_idx))`)
	require.NoError(t, err)
	for i, u := range passage.Units(raw, "Guide") {
		x := float32(0)
		switch text := raw[u.Start:u.End]; {
		case strings.Contains(text, "](#"):
			x = 1
		case strings.Contains(text, "Declare every option"):
			x = 0.9
		}
		_, err = db.Exec(`INSERT INTO passage_vectors VALUES ('guide', ?, ?, ?, ?)`, i, u.Start, u.End, index.EncodeEmbedding([]float32{x, 1 - x}))
		require.NoError(t, err)
	}

	item := askGuide(t, db, &passageMemoEmbedder{}, "zzz")

	for _, want := range tocGuideAnswer {
		assert.Contains(t, item.Body, want)
	}
	assert.NotContains(t, item.Body, "](#")
	assert.Equal(t, "dense", itemSelector(t, item))
}

// A pick with no heading section around it that is larger than the room is
// cut at a line, not dropped: a note that is one long list still delivers its
// opening items alongside a dense block.
func TestAsk_OverflowCutsAPickThatDoesNotFit(t *testing.T) {
	item := strings.Repeat("detail ", 120)
	list := "- First item " + item + "end.\n- Second item " + item + "end.\n- Third item " + item + "end.\n- Fourth item " + item + "end."
	db := tocGuideDB(t)
	raw := list + "\n\nThe closing line."
	_, err := db.Exec(`UPDATE notes SET body_raw=?, body_text=? WHERE id='guide'`, raw+"\n\n"+passageParagraph("unrelated", 400), raw)
	require.NoError(t, err)
	require.NoError(t, db.QueryRow(`SELECT body_raw FROM notes WHERE id='guide'`).Scan(&raw))
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS passage_vectors (note_id TEXT NOT NULL REFERENCES notes(id), unit_idx INTEGER NOT NULL, start INTEGER NOT NULL, end INTEGER NOT NULL, dense BLOB NOT NULL, PRIMARY KEY(note_id,unit_idx))`)
	require.NoError(t, err)
	for i, u := range passage.Units(raw, "Guide") {
		x := float32(0)
		if raw[u.Start:u.End] == "The closing line." {
			x = 1
		}
		_, err = db.Exec(`INSERT INTO passage_vectors VALUES ('guide', ?, ?, ?, ?)`, i, u.Start, u.End, index.EncodeEmbedding([]float32{x, 1 - x}))
		require.NoError(t, err)
	}

	got := askGuide(t, db, &passageMemoEmbedder{}, "zzz")

	assert.Contains(t, got.Body, "- First item")
	assert.Contains(t, got.Body, "The closing line.")
	assert.NotContains(t, got.Body, "Fourth item", "the room holds about two items")
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
