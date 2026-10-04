package importdocs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A path with "#" or "|" can't be a wikilink target (the parser reads them
// as a heading and an alias), so a book or long doc named with one links to
// its parts by note id instead.
func TestImport_AFileNameWithHashOrPipeLinksPartsByID(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	writeEPUB(t, filepath.Join(repo, "docs", "my#book|v2.epub"), twoChapterBook(true))
	write(t, filepath.Join(repo, "docs", "notes#draft.md"), longDoc("Install", "Configure", "Run", "Debug", "Extend", "Release", "Upgrade"))
	run(t, repo, vault, importdocs.Options{})

	dir := "imported/demo-repo/docs/my#book|v2-epub/"
	chapter, _ := noteAt(t, vault, dir+"01-chapter-one-arrival.md")
	_, book := noteAt(t, vault, dir+"book.md")
	assert.Contains(t, book, "[["+chapter["id"].(string)+"|Chapter One: Arrival]]")
	assert.NotContains(t, book, "[["+dir)

	section, _ := noteAt(t, vault, "imported/demo-repo/docs/notes#draft/01-install.md")
	index, err := os.ReadFile(filepath.Join(vault, "imported", "demo-repo", "docs", "notes#draft.md")) //nolint:gosec // test path
	require.NoError(t, err)
	assert.Contains(t, string(index), "[["+section["id"].(string)+"|")
	assert.False(t, strings.Contains(string(index), "[[imported/demo-repo/docs/notes#draft/"), "no path link with a #")
}
