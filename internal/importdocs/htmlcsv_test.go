package importdocs_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// importOne imports one file from the repo's docs folder and returns the
// note's frontmatter and body.
func importOne(t *testing.T, repo, name string) (map[string]interface{}, string) {
	t.Helper()
	vault := t.TempDir()
	res, err := importdocs.ImportFile(source(repo), name, vault, importdocs.Options{})
	require.NoError(t, err)
	require.Len(t, res.Entries, 1)
	require.Equal(t, importdocs.Added, res.Entries[0].Action, res.Entries[0].Reason)
	raw, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(res.Entries[0].Note))) //nolint:gosec // test path
	require.NoError(t, err)
	fm, body, err := parser.ExtractFrontmatter(raw)
	require.NoError(t, err)
	return fm, body
}

func TestImportFile_AnHTMLFileBecomesTheArticleItHolds(t *testing.T) {
	repo := srcRepo(t)
	var b strings.Builder
	b.WriteString(`<html><head><meta charset="iso-8859-1"><title>Install Guide</title></head><body>`)
	b.WriteString(`<nav><a href="index.html">Home</a> NAV-UNIQUE</nav><article><h1>Installing</h1>`)
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&b, "<p>Run the installer and pick a folder for the caf\xe9 files, step %d of the guide.</p>", i)
	}
	b.WriteString(`<p>See <a href="api/reference.html">the reference</a>.</p></article><footer>FOOTER-UNIQUE</footer></body></html>`)
	write(t, filepath.Join(repo, "docs", "install.html"), b.String())

	fm, body := importOne(t, repo, "install.html")
	assert.Equal(t, "Install Guide", fm["title"])
	assert.Equal(t, []interface{}{"demo-repo:docs/install.html"}, fm["paths"])
	assert.Contains(t, body, "café files", "the meta charset is honoured")
	assert.NotContains(t, body, "NAV-UNIQUE")
	assert.NotContains(t, body, "FOOTER-UNIQUE")
	assert.Contains(t, body, "(/docs/api/reference.html)", "a relative link points into the source, not at a web host")
	assert.NotContains(t, body, "localhost")
}

func TestImport_AFoldersHTMLAndCSVSitBesideItsMarkdown(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "guide.html"), "<html><body><main><h1>Guide</h1><p>"+strings.Repeat("Guide text. ", 40)+"</p></main></body></html>")
	write(t, filepath.Join(repo, "docs", "guide.md"), "# Guide in markdown\n")
	write(t, filepath.Join(repo, "docs", "prices.csv"), "item,price\ntea,3\n")
	write(t, filepath.Join(repo, "docs", "empty.htm"), "<html><body><script>app()</script></body></html>")

	res := run(t, repo, vault, importdocs.Options{})
	for _, want := range []string{"guide-html.md", "guide.md", "prices-csv.md"} {
		assert.FileExists(t, filepath.Join(vault, "imported", "demo-repo", "docs", want))
	}
	e, ok := entryFor(res, "empty.htm")
	require.True(t, ok)
	assert.Equal(t, importdocs.Skipped, e.Action)
	assert.Contains(t, e.Reason, "holds no text")
}

func TestImportFile_ACSVBecomesATable(t *testing.T) {
	cases := map[string]struct {
		name, content string
		want          []string
	}{
		"comma with a BOM": {"a.csv", "\ufeffname,city\nAda,London\n", []string{"| name | city |", "| Ada | London |"}},
		"semicolons":       {"b.csv", "name;city;note\nAda;London;x,y\nBo;Oslo;z\n", []string{"| name | city | note |", "| Ada | London | x,y |"}},
		"tsv":              {"c.tsv", "name\tcity\nAda\tLondon\n", []string{"| Ada | London |"}},
		"quoted and ragged": {"d.csv", "name,quote\nAda,\"said \"\"hi\"\", then left\"\nBo\n",
			[]string{`| Ada | said "hi", then left |`, "| Bo |  |"}},
		"latin-1": {"e.csv", "name,city\nJos\xe9,M\xe1laga\n", []string{"| José | Málaga |"}},
		"pipes":   {"f.csv", "expr\na|b\n", []string{`| a\|b |`}},
		// Excel exports trailing rows of empty cells; they are not rows.
		"blank rows": {"g.csv", "name,city\n,\nAda,London\n,,\n", []string{"| name | city |\n| --- | --- |\n| Ada | London |\n\n"}},
	}
	for label, c := range cases {
		t.Run(label, func(t *testing.T) {
			repo := srcRepo(t)
			write(t, filepath.Join(repo, "docs", c.name), c.content)
			_, body := importOne(t, repo, c.name)
			for _, w := range c.want {
				assert.Contains(t, body, w)
			}
			assert.NotContains(t, body, "\ufeff")
		})
	}
}

func TestImportFile_ABigCSVShowsTheCapsAndSaysWhatIsLeftOut(t *testing.T) {
	repo := srcRepo(t)
	var b strings.Builder
	cols := make([]string, 60)
	for i := range cols {
		cols[i] = fmt.Sprintf("c%d", i)
	}
	b.WriteString(strings.Join(cols, ",") + "\n")
	for r := 0; r < 250; r++ {
		row := make([]string, 60)
		for i := range row {
			row[i] = fmt.Sprintf("r%dc%d", r, i)
		}
		b.WriteString(strings.Join(row, ",") + "\n")
	}
	write(t, filepath.Join(repo, "docs", "big.csv"), b.String())

	_, body := importOne(t, repo, "big.csv")
	assert.Contains(t, body, "r199c49")
	assert.NotContains(t, body, "r200c0", "200 rows after the header")
	assert.NotContains(t, body, "c50", "50 columns")
	assert.Contains(t, body, "(50 more rows not shown)")
	assert.Contains(t, body, "(10 more columns not shown)")
}
