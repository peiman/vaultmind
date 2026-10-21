package cmd

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A merge-conflict marker left in a file still builds and still passes every
// other test when the file is not code. 0.10.17's follow-up (#340) merged a
// CHANGELOG with both sides of a conflict and the markers between them: a
// stash pop had conflicted and the commit step carried on. Nothing failed.
func TestNoMergeConflictMarkers(t *testing.T) {
	textExt := map[string]bool{".go": true, ".md": true, ".sh": true, ".yml": true, ".yaml": true, ".json": true, ".toml": true, ".txt": true}
	skipDir := map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true}
	var scanned int
	var hits []string
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !textExt[filepath.Ext(path)] {
			return nil
		}
		scanned++
		hits = append(hits, conflictMarkerLines(t, path)...)
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, scanned, "no text files scanned: the walk root is wrong")
	require.Empty(t, hits, "merge-conflict markers left in files; resolve them:\n%s", strings.Join(hits, "\n"))
}

func conflictMarkerLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path) // #nosec G304 -- walking this repository's own files in a test
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	var hits []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if strings.HasPrefix(line, "<<<<<<< ") || strings.HasPrefix(line, ">>>>>>> ") {
			hits = append(hits, path+":"+strconv.Itoa(n)+": "+line)
		}
	}
	require.NoError(t, sc.Err(), path)
	return hits
}
