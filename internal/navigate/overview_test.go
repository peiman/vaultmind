package navigate_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/navigate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A folder's overview note (type: overview) gives the folder its one-line
// summary; it is the folder's description, not one of its notes.
func TestBuild_AnOverviewNoteSummarizesItsFolder(t *testing.T) {
	notes := []navigate.Note{
		{ID: "ov", Path: "concepts/overview.md", Title: "Concepts", Type: "overview", Line: "Memory and retrieval research."},
		{ID: "a", Path: "concepts/a.md", Title: "A", Type: "concept"},
		{ID: "b", Path: "concepts/b.md", Title: "B", Type: "concept"},
	}
	root := navigate.Build(notes)
	require.Len(t, root.Dirs, 1)
	d := root.Dirs[0]
	assert.Equal(t, "Memory and retrieval research.", d.Summary)
	assert.Equal(t, "overview", d.SummaryFrom)
	assert.Equal(t, 2, d.Total, "the overview is not counted as one of the folder's notes")
	assert.Len(t, d.Notes, 2, "nor listed among them")

	out := render(t, root, navigate.RenderOptions{Depth: 1})
	assert.Contains(t, out, "concepts/ (2) — Memory and retrieval research.\n")
}

// Without an overview, a folder is described by its most used tags, the
// notes beneath it included; with neither, it is a plain count.
func TestBuild_FallsBackToTheFoldersTopTags(t *testing.T) {
	notes := []navigate.Note{
		{ID: "a", Path: "sources/a.md", Tags: []string{"sleep", "memory"}},
		{ID: "b", Path: "sources/b.md", Tags: []string{"sleep"}},
		{ID: "c", Path: "sources/deep/c.md", Tags: []string{"sleep", "rag"}},
		{ID: "d", Path: "people/d.md"},
	}
	root := navigate.Build(notes)
	out := render(t, root, navigate.RenderOptions{Depth: 1})
	assert.Contains(t, out, "sources/ (3) — tags: sleep 3, memory 1, rag 1\n")
	assert.Contains(t, out, "people/ (1)\n")
	assert.Equal(t, "tags", root.Dirs[1].SummaryFrom)

	raw, err := json.Marshal(root.Dirs[1])
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"summary":"tags: sleep 3, memory 1, rag 1"`)
}

// Only the five most used tags are named.
func TestBuild_NamesAtMostFiveTags(t *testing.T) {
	notes := []navigate.Note{{ID: "a", Path: "x/a.md", Tags: []string{"t1", "t2", "t3", "t4", "t5", "t6"}}}
	out := render(t, navigate.Build(notes), navigate.RenderOptions{Depth: 1})
	assert.Contains(t, out, "x/ (1) — tags: t1 1, t2 1, t3 1, t4 1, t5 1\n")
}

// An overview's summary is its first sentence up to 256 characters (an L0
// abstract), not cut at a note line's 120: the probe measured whole
// overview lines, and the cut dropped half of each.
func TestLoad_AnOverviewKeepsALongerSummary(t *testing.T) {
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	long := "Concepts spanning memory and neuroscience, sleep stages, consolidation, dreaming, neurons and optogenetics, and machine learning with transformers, retrieval, embeddings, agents and training."
	require.Greater(t, len(long), navigate.MaxLineRunes)
	_, err = db.Exec(`INSERT INTO notes (id, path, title, type, body_raw, hash, mtime) VALUES ('ov', 'c/c-overview.md', 'C', 'overview', ?, 'h', 1), ('n', 'c/n.md', 'N', 'concept', ?, 'h', 1)`, long, long)
	require.NoError(t, err)

	notes, err := navigate.Load(db, navigate.Filter{})
	require.NoError(t, err)
	byID := map[string]string{}
	for _, n := range notes {
		byID[n.ID] = n.Line
	}
	assert.Equal(t, long, byID["ov"], "the overview's whole first sentence")
	assert.LessOrEqual(t, len([]rune(byID["n"])), navigate.MaxLineRunes, "an ordinary note keeps the short line")
}

// Load reads each note's tags and modification time from the index.
func TestLoad_ReadsTagsAndMtime(t *testing.T) {
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`INSERT INTO notes (id, path, title, type, body_raw, hash, mtime) VALUES ('a', 'x/a.md', 'A', 'concept', 'Body.', 'h', 42)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO tags (note_id, tag) VALUES ('a', 'sleep'), ('a', 'memory')`)
	require.NoError(t, err)

	notes, err := navigate.Load(db, navigate.Filter{})
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.ElementsMatch(t, []string{"sleep", "memory"}, notes[0].Tags)
	assert.Equal(t, int64(42), notes[0].Mtime)
}
