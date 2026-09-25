package navigate_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/navigate"
	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An agent cannot use a knowledge base it cannot see. `tree` is the map: what
// a vault holds, by folder, one line per note, without guessing a query.

func sample() []navigate.Note {
	return []navigate.Note{
		{ID: "concept-b", Path: "concepts/b.md", Title: "Bee", Type: "concept", Line: "About b."},
		{ID: "concept-a", Path: "concepts/a.md", Title: "Ay", Type: "concept", Line: "About a."},
		{ID: "decision-x", Path: "decisions/2026/x.md", Title: "Ex", Type: "decision", Line: "Chose x."},
		{ID: "readme-ish", Path: "top.md", Title: "Top", Type: "note", Line: "At the root."},
	}
}

func render(t *testing.T, root *navigate.Dir, o navigate.RenderOptions) string {
	t.Helper()
	var b strings.Builder
	require.NoError(t, navigate.Render(&b, root, o))
	return b.String()
}

func TestBuild_GroupsByFolderWithRecursiveCounts(t *testing.T) {
	root := navigate.Build(sample())

	assert.Equal(t, 4, root.Total)
	require.Len(t, root.Dirs, 2)
	assert.Equal(t, "concepts", root.Dirs[0].Path, "folders sort by path")
	assert.Equal(t, 2, root.Dirs[0].Total)
	assert.Equal(t, "decisions", root.Dirs[1].Path)
	assert.Equal(t, 1, root.Dirs[1].Total, "a folder counts notes in its subfolders too")
	require.Len(t, root.Notes, 1, "a note at the root stays at the root")
	assert.Equal(t, "Ay", root.Dirs[0].Notes[0].Title, "notes sort by path within a folder")
}

func TestRender_ShowsEveryNoteWithItsIDAndLine(t *testing.T) {
	out := render(t, navigate.Build(sample()), navigate.RenderOptions{})

	assert.Contains(t, out, "concepts/ (2)")
	assert.Contains(t, out, "decisions/2026/ (1)")
	assert.Contains(t, out, "Ay (concept-a) — About a.", "title, the id to open it with, and what it is about")
	assert.Contains(t, out, "Top (readme-ish) — At the root.")
}

func TestRender_DepthCollapsesDeeperFoldersIntoCounts(t *testing.T) {
	out := render(t, navigate.Build(sample()), navigate.RenderOptions{Depth: 1})

	assert.Contains(t, out, "concepts/ (2)")
	assert.Contains(t, out, "decisions/ (1)")
	assert.NotContains(t, out, "decisions/2026/", "folders below the depth are counted, not listed")
	assert.NotContains(t, out, "About a.", "notes inside a collapsed folder are not listed")
	assert.Contains(t, out, "Top (readme-ish)", "notes at the root are within depth 1")
}

func TestRender_BriefDropsTheLines(t *testing.T) {
	out := render(t, navigate.Build(sample()), navigate.RenderOptions{Brief: true})

	assert.Contains(t, out, "Ay (concept-a)")
	assert.NotContains(t, out, "About a.")
}

func TestOneLine_TakesTheFirstSentenceOfTheExcerpt(t *testing.T) {
	body := "---\nid: x\ntype: concept\n---\n\n# Heading\n\nFirst sentence here. Second sentence is not the line.\n"
	assert.Equal(t, "First sentence here.", navigate.OneLine(body))
}

func TestOneLine_PrefersThePrincipleSection(t *testing.T) {
	body := "Some story first. More story.\n\n## Principle\n\nAlways measure first. Then decide.\n"
	assert.Equal(t, "Always measure first.", navigate.OneLine(body))
}

func TestOneLine_CapsALongSentence(t *testing.T) {
	line := navigate.OneLine(strings.Repeat("word ", 80) + ".")
	assert.LessOrEqual(t, len([]rune(line)), navigate.MaxLineRunes)
	assert.True(t, strings.HasSuffix(line, "…"), "a cut line says it was cut")
}

func TestLoad_ReadsTheIndexAndFilters(t *testing.T) {
	vault := testvault.FixtureVault()
	db := testvault.OpenSharedDB(t, vault, filepath.Join(t.TempDir(), "index.db"))
	defer func() { _ = db.Close() }()

	all, err := navigate.Load(db, navigate.Filter{})
	require.NoError(t, err)
	require.NotEmpty(t, all)
	var actr navigate.Note
	for _, n := range all {
		if n.ID == "concept-act-r" {
			actr = n
		}
	}
	assert.Equal(t, "ACT-R", actr.Title)
	assert.Equal(t, "concepts/act-r.md", actr.Path)
	assert.NotEmpty(t, actr.Line, "every note gets a line from its body")

	decisions, err := navigate.Load(db, navigate.Filter{Type: "decision"})
	require.NoError(t, err)
	require.NotEmpty(t, decisions)
	for _, n := range decisions {
		assert.Equal(t, "decision", n.Type)
	}

	concepts, err := navigate.Load(db, navigate.Filter{PathPrefix: "concepts/"})
	require.NoError(t, err)
	require.NotEmpty(t, concepts)
	for _, n := range concepts {
		assert.True(t, strings.HasPrefix(n.Path, "concepts/"), n.Path)
	}
}
