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

// longDoc is markdown past the long-doc threshold, one ~5,000-char section
// per heading.
func longDoc(headings ...string) string {
	var b strings.Builder
	b.WriteString("# The Manual\n\n")
	for _, h := range headings {
		word := strings.ToLower(h)
		b.WriteString("## " + h + "\n\n" + strings.Repeat(word+" text here. ", 400) + "\n\n")
	}
	return b.String()
}

func entries(res *importdocs.Result, a importdocs.Action) []string {
	var out []string
	for _, e := range res.Entries {
		if e.Action == a {
			out = append(out, e.Note)
		}
	}
	return out
}

// A long doc becomes an index note at its usual path plus a folder of
// section notes; a re-run changes nothing; a change to one section updates
// only that section.
func TestImport_ALongDocBecomesAnIndexAndSections(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	long := filepath.Join(repo, "docs", "manual.md")
	write(t, long, longDoc("Install", "Configure", "Run", "Debug", "Extend", "Release", "Upgrade"))

	res := run(t, repo, vault, importdocs.Options{})
	dir := filepath.Join(vault, "imported", "demo-repo", "docs")
	idx, err := os.ReadFile(filepath.Join(dir, "manual.md")) //nolint:gosec // test path
	require.NoError(t, err)
	assert.Contains(t, string(idx), "id: imported-demo-repo-docs-manual\n")
	assert.Contains(t, string(idx), "[[imported/demo-repo/docs/manual/01-install|", "sections are linked by path, which the import fixes")
	sec, err := os.ReadFile(filepath.Join(dir, "manual", "01-install.md")) //nolint:gosec // test path
	require.NoError(t, err)
	assert.Contains(t, string(sec), "[[imported/demo-repo/docs/manual]]", "a section links back to its index")
	assert.Contains(t, string(sec), "source: demo-repo:docs/manual.md#01-install\n")
	assert.NotContains(t, string(sec), "paths:", "only the index is tied to the doc, so opening it brings one note")
	assert.GreaterOrEqual(t, len(entries(res, importdocs.Added)), 9, "2 short docs, the index and 7 sections")

	again := run(t, repo, vault, importdocs.Options{})
	assert.Empty(t, entries(again, importdocs.Added))
	assert.Empty(t, entries(again, importdocs.Updated), "a re-run changes nothing")

	raw, err := os.ReadFile(long) //nolint:gosec // test path
	require.NoError(t, err)
	// A sentence deep in the section: its first line, which the index lists,
	// stays the same.
	at := strings.LastIndex(string(raw), "debug text here.")
	write(t, long, string(raw[:at])+"debug text, revised."+string(raw[at+len("debug text here."):]))
	changed := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, []string{"imported/demo-repo/docs/manual/04-debug.md"}, entries(changed, importdocs.Updated))
}

// When a doc shrinks, the sections it no longer has are orphaned, and
// --prune removes them; when it is no longer long, all its sections are.
func TestImport_SectionsADocNoLongerHasAreOrphaned(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	long := filepath.Join(repo, "docs", "manual.md")
	write(t, long, longDoc("Install", "Configure", "Run", "Debug", "Extend", "Release", "Upgrade"))
	run(t, repo, vault, importdocs.Options{})

	write(t, long, longDoc("Install", "Configure", "Run", "Debug", "Extend", "Release"))
	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, []string{"imported/demo-repo/docs/manual/07-upgrade.md"}, entries(res, importdocs.Orphaned))

	pruned := run(t, repo, vault, importdocs.Options{Prune: true})
	assert.Equal(t, []string{"imported/demo-repo/docs/manual/07-upgrade.md"}, entries(pruned, importdocs.Pruned))
	assert.NoFileExists(t, filepath.Join(vault, "imported", "demo-repo", "docs", "manual", "07-upgrade.md"))

	write(t, long, "# The Manual\n\nShort now.\n")
	short := run(t, repo, vault, importdocs.Options{})
	assert.Len(t, entries(short, importdocs.Orphaned), 6, "every section of a doc that is no longer long")
}

// A long EPUB chapter or archive member splits too, its sections keeping
// the whole chain of parts in their source.
func TestImport_ALongMemberOfAnArchiveSplitsToo(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "bundle.zip"), string(zipBytes(t, []member{
		{name: "manual.md", body: longDoc("Install", "Configure", "Run", "Debug", "Extend", "Release", "Upgrade")}})))
	run(t, repo, vault, importdocs.Options{})
	sec, err := os.ReadFile(filepath.Join(vault, "imported", "demo-repo", "docs", "bundle-zip", "manual", "03-run.md")) //nolint:gosec // test path
	require.NoError(t, err)
	assert.Contains(t, string(sec), "source: demo-repo:docs/bundle.zip#manual.md#03-run\n")
}

// A page re-imported shorter orphans its stale sections too, though a page
// import has no folder to compare against.
func TestImportURL_StaleSectionsOfAPageAreOrphaned(t *testing.T) {
	vault := t.TempDir()
	page := func(headings ...string) importdocs.Page {
		var b strings.Builder
		b.WriteString("<html><head><title>Manual</title></head><body><article><h1>Manual</h1>")
		for _, h := range headings {
			b.WriteString("<h2>" + h + "</h2>")
			// ~5,400 chars a section, whatever the heading: under the split
			// size, and seven of them past the long threshold.
			for i := 0; i < 12; i++ {
				b.WriteString("<p>" + strings.Repeat(strings.ToLower(h)[:3]+" words here. ", 30) + "</p>")
			}
		}
		b.WriteString("</article></body></html>")
		return importdocs.Page{FinalURL: "https://example.com/manual", ContentType: "text/html", Body: []byte(b.String())}
	}
	importPage(t, "https://example.com/manual", vault, importdocs.Options{}, pageFetcher(page("Install", "Configure", "Run", "Debug", "Extend", "Release", "Upgrade")))
	require.FileExists(t, filepath.Join(vault, "imported", "web", "example.com", "manual", "07-upgrade.md"))

	res := importPage(t, "https://example.com/manual", vault, importdocs.Options{}, pageFetcher(page("Install", "Configure", "Run", "Debug", "Extend", "Release")))
	assert.Equal(t, []string{"imported/web/example.com/manual/07-upgrade.md"}, entries(res, importdocs.Orphaned))
}

// A section edited by hand is a conflict, as any imported note is.
func TestImport_AHandEditedSectionIsAConflict(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	long := filepath.Join(repo, "docs", "manual.md")
	write(t, long, longDoc("Install", "Configure", "Run", "Debug", "Extend", "Release", "Upgrade"))
	run(t, repo, vault, importdocs.Options{})

	sec := filepath.Join(vault, "imported", "demo-repo", "docs", "manual", "02-configure.md")
	raw, err := os.ReadFile(sec) //nolint:gosec // test path
	require.NoError(t, err)
	write(t, sec, string(raw)+"\nMy own note.\n")
	srcRaw, err := os.ReadFile(long) //nolint:gosec // test path
	require.NoError(t, err)
	write(t, long, strings.Replace(string(srcRaw), "configure text here.", "configure, changed.", 1))

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, []string{"imported/demo-repo/docs/manual/02-configure.md"}, entries(res, importdocs.Conflict))
}
