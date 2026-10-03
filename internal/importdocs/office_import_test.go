package importdocs_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeDocx writes a one-paragraph Word document.
func writeDocx(t *testing.T, path, text string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	require.NoError(t, err)
	_, err = w.Write([]byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	write(t, path, buf.String())
}

// A folder's Office files import beside its markdown, as <name>-docx.md tied
// to the file by paths:; a broken one is skipped with its reason and the rest
// still imports.
func TestImport_AFoldersOfficeFilesBecomeNotesAndABrokenOneIsSkipped(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	writeDocx(t, filepath.Join(repo, "docs", "plan.docx"), "The plan in Word.")
	write(t, filepath.Join(repo, "docs", "broken.xlsx"), "not a zip")

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 3, res.Count(importdocs.Added), "two markdown docs and the Word file")

	raw, err := os.ReadFile(filepath.Join(vault, "imported", "demo-repo", "docs", "plan-docx.md")) //nolint:gosec // test path
	require.NoError(t, err)
	fm, body, err := parser.ExtractFrontmatter(raw)
	require.NoError(t, err)
	assert.Equal(t, []interface{}{"demo-repo:docs/plan.docx"}, fm["paths"])
	assert.Equal(t, "plan", fm["title"], "no title in the file, so its name")
	assert.Contains(t, body, "The plan in Word.")

	e, ok := entryFor(res, "broken.xlsx")
	require.True(t, ok, "the broken file is reported")
	assert.Equal(t, importdocs.Skipped, e.Action)
	assert.Contains(t, e.Reason, "not a readable xlsx file")
}

// One Office file imports alone.
func TestImportFile_AnOfficeFileImportsAlone(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	writeDocx(t, filepath.Join(repo, "docs", "plan.docx"), "Alone.")

	res, err := importdocs.ImportFile(source(repo), "plan.docx", vault, importdocs.Options{})
	require.NoError(t, err)
	require.Len(t, res.Entries, 1)
	assert.Equal(t, "imported/demo-repo/docs/plan-docx.md", res.Entries[0].Note)
}
