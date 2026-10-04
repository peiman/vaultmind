package importdocs

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeZip(t *testing.T, p string, files map[string]string) {
	t.Helper()
	require.NotEmpty(t, files, "an archive needs members")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(p, buf.Bytes(), 0o600))
}

// tempRoot points the archive's temporary folder at a test directory and
// returns it, so a test can see that nothing is left behind.
func tempRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := archiveTempRoot
	t.Cleanup(func() { archiveTempRoot = prev })
	archiveTempRoot = dir
	return dir
}

func leftovers(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestReadArchive_RefusesAMemberPastItsCapByBytesWritten(t *testing.T) {
	root := tempRoot(t)
	prev := maxArchiveMember
	t.Cleanup(func() { maxArchiveMember = prev })
	maxArchiveMember = 100
	p := filepath.Join(t.TempDir(), "bomb.zip")
	writeZip(t, p, map[string]string{"big.md": strings.Repeat("a", 101), "small.md": "# Small\n"})

	_, _, err := readArchiveDocs(Source{Repo: "r"}, "bomb.zip", p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "big.md is larger than")
	assert.Empty(t, leftovers(t, root), "the temporary folder is removed on error too")
}

func TestReadArchive_RefusesPastTheTotalAndMemberCountCaps(t *testing.T) {
	tempRoot(t)
	prevTotal, prevCount := maxArchiveTotal, maxArchiveMembers
	t.Cleanup(func() { maxArchiveTotal, maxArchiveMembers = prevTotal, prevCount })
	p := filepath.Join(t.TempDir(), "many.zip")
	writeZip(t, p, map[string]string{"a.md": strings.Repeat("a", 60), "b.md": strings.Repeat("b", 60), "c.md": "c"})

	maxArchiveTotal = 100
	_, _, err := readArchiveDocs(Source{Repo: "r"}, "many.zip", p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than")

	maxArchiveTotal, maxArchiveMembers = prevTotal, 2
	_, _, err = readArchiveDocs(Source{Repo: "r"}, "many.zip", p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than 2 members")
}

func TestReadArchive_LeavesNoTemporaryFolderBehind(t *testing.T) {
	root := tempRoot(t)
	p := filepath.Join(t.TempDir(), "ok.zip")
	writeZip(t, p, map[string]string{"a.md": "# A\n"})
	docs, _, err := readArchiveDocs(Source{Repo: "r"}, "ok.zip", p)
	require.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Empty(t, leftovers(t, root))
}

func TestSafeMemberPath(t *testing.T) {
	dir := t.TempDir()
	ok := map[string]bool{
		"a.md": true, "docs/a.md": true, "./docs/a.md": true, "docs/../a.md": true,
		"../a.md": false, "a/../../b.md": false, "/abs.md": false, "..": false, ".": false, "": false,
		`dir\evil.md`: false, "a\x00b.md": false, "a\nb.md": false,
	}
	got := map[string]bool{}
	for name := range ok {
		dest, fine := safeMemberPath(dir, name)
		if fine {
			assert.True(t, strings.HasPrefix(dest, dir+string(filepath.Separator)), name)
		}
		got[name] = fine
	}
	assert.Equal(t, ok, got)
}

// Hidden and dependency folders are not even written: a zipped project can
// carry a node_modules of thousands of files.
func TestExtractArchive_WritesNoHiddenOrDependencyFolder(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(t.TempDir(), "proj.zip")
	writeZip(t, p, map[string]string{"proj/README.md": "# R\n", "proj/.github/notes.md": "x", "proj/node_modules/lib/README.md": "x"})
	_, err := extractArchive(p, dir)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "proj", "README.md"))
	assert.NoDirExists(t, filepath.Join(dir, "proj", ".github"))
	assert.NoDirExists(t, filepath.Join(dir, "proj", "node_modules"))
}
