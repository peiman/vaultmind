package importdocs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// copyFixture puts a PDF from testdata into the source folder.
func copyFixture(t *testing.T, name, dst string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name)) //nolint:gosec // test fixture
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o750))
	require.NoError(t, os.WriteFile(dst, b, 0o600))
}

func entryFor(res *importdocs.Result, note string) (importdocs.Entry, bool) {
	for _, e := range res.Entries {
		if e.Note == note {
			return e, true
		}
	}
	return importdocs.Entry{}, false
}

// A folder's PDFs import beside its markdown: the text PDF becomes
// <name>-pdf.md, tied to the PDF by paths:, and a scan is skipped with its
// reason while everything else is still written.
func TestImport_AFoldersPDFsBecomeNotesAndAScanIsSkipped(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	copyFixture(t, "paper.pdf", filepath.Join(repo, "docs", "paper.pdf"))
	copyFixture(t, "no-text.pdf", filepath.Join(repo, "docs", "scan.pdf"))

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 3, res.Count(importdocs.Added), "two markdown docs and the text PDF")

	raw, err := os.ReadFile(filepath.Join(vault, "imported", "demo-repo", "docs", "paper-pdf.md")) //nolint:gosec // test path
	require.NoError(t, err)
	fm, body, err := parser.ExtractFrontmatter(raw)
	require.NoError(t, err)
	assert.Equal(t, "Retrieval for Agent Memory: A Study", fm["title"])
	assert.Equal(t, []interface{}{"demo-repo:docs/paper.pdf"}, fm["paths"], "reading the PDF brings the note")
	assert.Contains(t, body, "We study embedding long notes")

	var scan importdocs.Entry
	for _, e := range res.Entries {
		if e.Action == importdocs.Skipped && filepath.Base(e.Note) == "scan.pdf" {
			scan = e
		}
	}
	assert.Contains(t, scan.Reason, "no text layer", "the scan is reported, not imported empty")
	assert.NoFileExists(t, filepath.Join(vault, "imported", "demo-repo", "docs", "scan-pdf.md"))
}

// paper.md and paper.pdf in one folder are two notes.
func TestImport_AMarkdownAndAPDFOfOneNameDoNotCollide(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "paper.md"), "# Paper notes\n\nThoughts on the paper.\n")
	copyFixture(t, "paper.pdf", filepath.Join(repo, "docs", "paper.pdf"))

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 4, res.Count(importdocs.Added))
	assert.FileExists(t, filepath.Join(vault, "imported", "demo-repo", "docs", "paper.md"))
	assert.FileExists(t, filepath.Join(vault, "imported", "demo-repo", "docs", "paper-pdf.md"))
}

func TestImport_APDFsRerunAndChangeFollowItsText(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	pdf := filepath.Join(repo, "docs", "paper.pdf")
	copyFixture(t, "paper.pdf", pdf)
	run(t, repo, vault, importdocs.Options{})

	res := run(t, repo, vault, importdocs.Options{})
	e, ok := entryFor(res, "imported/demo-repo/docs/paper-pdf.md")
	require.True(t, ok)
	assert.Equal(t, importdocs.Unchanged, e.Action)

	copyFixture(t, "placeholder-title.pdf", pdf)
	res = run(t, repo, vault, importdocs.Options{})
	e, _ = entryFor(res, "imported/demo-repo/docs/paper-pdf.md")
	assert.Equal(t, importdocs.Updated, e.Action, "new text, new content")
}

func TestImport_AnUnreadablePDFIsSkippedNotFatal(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "broken.pdf"), "%PDF-1.4 truncated")

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 2, res.Count(importdocs.Added), "the markdown still imports")
	found := false
	for _, e := range res.Entries {
		if e.Action == importdocs.Skipped && filepath.Base(e.Note) == "broken.pdf" {
			found = true
			assert.Contains(t, e.Reason, "reading the PDF")
		}
	}
	assert.True(t, found)
}

// A PDF that imported once and now cannot be read is skipped, and its note
// stays: --prune removes the notes of docs that are gone, and this one is not.
func TestImport_APDFThatBecameUnreadableKeepsItsNote(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	pdf := filepath.Join(repo, "docs", "paper.pdf")
	copyFixture(t, "paper.pdf", pdf)
	run(t, repo, vault, importdocs.Options{})
	write(t, pdf, "%PDF-1.4 truncated")

	res := run(t, repo, vault, importdocs.Options{Prune: true})
	assert.FileExists(t, filepath.Join(vault, "imported", "demo-repo", "docs", "paper-pdf.md"))
	assert.Zero(t, res.Count(importdocs.Orphaned))
	assert.Zero(t, res.Count(importdocs.Pruned))
}

// A PDF deleted from the folder orphans its note, as a markdown doc does, and
// --prune removes it.
func TestImport_ADeletedPDFOrphansItsNote(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	pdf := filepath.Join(repo, "docs", "paper.pdf")
	copyFixture(t, "paper.pdf", pdf)
	run(t, repo, vault, importdocs.Options{})
	require.NoError(t, os.Remove(pdf))

	res := run(t, repo, vault, importdocs.Options{})
	e, ok := entryFor(res, "imported/demo-repo/docs/paper-pdf.md")
	require.True(t, ok)
	assert.Equal(t, importdocs.Orphaned, e.Action)

	res = run(t, repo, vault, importdocs.Options{Prune: true})
	e, _ = entryFor(res, "imported/demo-repo/docs/paper-pdf.md")
	assert.Equal(t, importdocs.Pruned, e.Action)
	assert.NoFileExists(t, filepath.Join(vault, "imported", "demo-repo", "docs", "paper-pdf.md"))
}

// One file imports alone, with the repository and prefix of its folder, and
// without an orphan pass: a sibling's note is not called orphaned.
func TestImportFile_ImportsOneFileAndLeavesSiblingsAlone(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	run(t, repo, vault, importdocs.Options{}) // alpha and beta are notes
	copyFixture(t, "paper.pdf", filepath.Join(repo, "docs", "paper.pdf"))

	res, err := importdocs.ImportFile(source(repo), "paper.pdf", vault, importdocs.Options{Prune: true})
	require.NoError(t, err)
	require.Len(t, res.Entries, 1)
	assert.Equal(t, importdocs.Added, res.Entries[0].Action)
	assert.Equal(t, "imported/demo-repo/docs/paper-pdf.md", res.Entries[0].Note)
	assert.FileExists(t, filepath.Join(vault, "imported", "demo-repo", "docs", "alpha.md"), "--prune touches nothing else")

	res, err = importdocs.ImportFile(source(repo), "alpha.md", vault, importdocs.Options{})
	require.NoError(t, err)
	require.Len(t, res.Entries, 1)
	assert.Equal(t, importdocs.Unchanged, res.Entries[0].Action)
}

// A PDF over the 20 MiB cap is skipped before it is read into memory.
func TestImport_AnOversizedPDFIsSkipped(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	big := filepath.Join(repo, "docs", "huge.pdf")
	f, err := os.Create(big) //nolint:gosec // test path
	require.NoError(t, err)
	require.NoError(t, f.Truncate(20<<20+1))
	require.NoError(t, f.Close())

	res := run(t, repo, vault, importdocs.Options{})
	found := false
	for _, e := range res.Entries {
		if e.Action == importdocs.Skipped && filepath.Base(e.Note) == "huge.pdf" {
			found = true
			assert.Contains(t, e.Reason, "larger than 20 MiB")
		}
	}
	assert.True(t, found)
}

// ImportFile names a file in the source folder, nothing else.
func TestImportFile_RefusesWhatIsNotAFileInTheFolder(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	for _, rel := range []string{"sub/beta.md", "../escape.md", "missing.md"} {
		_, err := importdocs.ImportFile(source(repo), rel, vault, importdocs.Options{})
		assert.Error(t, err, rel)
	}
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "docs", "folder.md"), 0o750))
	_, err := importdocs.ImportFile(source(repo), "folder.md", vault, importdocs.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")

	bad := source(repo)
	bad.Repo = "../x"
	_, err = importdocs.ImportFile(bad, "alpha.md", vault, importdocs.Options{})
	assert.Error(t, err, "the repository name is validated as for a folder import")
}

func TestImportFile_RefusesOtherFiles(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	_, err := importdocs.ImportFile(source(repo), "image.png", vault, importdocs.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".md, .pdf, .docx, .pptx, .xlsx, .html, .htm, .csv or .tsv")

	write(t, filepath.Join(repo, "docs", "scan.pdf"), "%PDF-1.4 truncated")
	_, err = importdocs.ImportFile(source(repo), "scan.pdf", vault, importdocs.Options{})
	require.Error(t, err, "a single PDF that cannot be read is an error, not a silent skip")
}
