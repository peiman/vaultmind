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

// Every entry counts toward the member cap, folders and links too: a zip of
// a million empty folders is refused like a zip of a million files.
func TestReadArchive_FoldersCountTowardTheMemberCap(t *testing.T) {
	tempRoot(t)
	prev := maxArchiveMembers
	t.Cleanup(func() { maxArchiveMembers = prev })
	maxArchiveMembers = 3
	p := filepath.Join(t.TempDir(), "dirs.zip")
	writeZip(t, p, map[string]string{"d1/": "", "d2/": "", "d3/": "", "a.md": "# A\n"})
	_, _, err := readArchiveDocs(Source{Repo: "r"}, "dirs.zip", p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than 3 members")
}

// The total cap holds before a byte past it is written: the member that
// would cross it is cut at the room left, then refused.
func TestExtractArchive_NeverWritesPastTheTotalCap(t *testing.T) {
	prev := maxArchiveTotal
	t.Cleanup(func() { maxArchiveTotal = prev })
	maxArchiveTotal = 100
	dir := t.TempDir()
	p := filepath.Join(t.TempDir(), "two.zip")
	writeZip(t, p, map[string]string{"a.md": strings.Repeat("a", 60)})
	x := &extractor{dir: dir, written: 60}
	err := x.write("b.md", filepath.Join(dir, "b.md"), strings.NewReader(strings.Repeat("b", 60)))
	require.Error(t, err)
	info, statErr := os.Stat(filepath.Join(dir, "b.md"))
	require.NoError(t, statErr)
	assert.LessOrEqual(t, info.Size(), int64(41), "at most the room left, plus the byte that shows it ran over")
}

// On a filesystem that folds case, A.md and a.md are one file: the second
// is reported as a name clash, not as a raw error.
func TestExtractArchive_ACaseClashIsReportedPlainly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Probe"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "probe")); err != nil {
		t.Skip("this filesystem tells case apart")
	}
	p := filepath.Join(t.TempDir(), "case.zip")
	writeZip(t, p, map[string]string{"A.md": "# A\n", "a.md": "# a\n"})
	skipped, err := extractArchive(p, filepath.Join(dir, "out"))
	require.NoError(t, err)
	require.Len(t, skipped, 1)
	assert.Contains(t, skipped[0].Reason, "another member has the same name")
}
