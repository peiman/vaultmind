package cmd

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The CLI path of the symlink wipe: index, ask, search, doctor and note get
// all take --vault, and a pointer such as vaultmind-identity must name the
// real directory for each of them.
func TestCommands_SymlinkedVaultIndexesAndReads(t *testing.T) {
	isolateMeshEnv(t)
	t.Setenv("VAULTMIND_NO_UPDATE_CHECK", "1")

	vault := buildIndexedTestVault(t)
	link := filepath.Join(t.TempDir(), "current")
	require.NoError(t, os.Symlink(vault, link))

	out, _, err := runRootCmd(t, "index", "--vault", link, "--json")
	require.NoError(t, err)
	var indexed struct {
		Status string `json:"status"`
		Result struct {
			Index struct {
				Skipped int `json:"skipped"`
				Indexed int `json:"indexed"`
				Deleted int `json:"deleted"`
			} `json:"index"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &indexed))
	assert.Equal(t, "ok", indexed.Status)
	assert.Equal(t, 0, indexed.Result.Index.Deleted)
	seen := indexed.Result.Index.Skipped + indexed.Result.Index.Indexed
	assert.Positive(t, seen, "index through the symlink must see the notes, not an empty vault")

	ask, _, err := runRootCmd(t, "ask", "Alpha Concept", "--vault", link, "--json",
		"--budget", "1500", "--max-items", "3", "--search-limit", "5")
	require.NoError(t, err)
	assert.Contains(t, ask.String(), "concept-alpha")

	search, _, err := runRootCmd(t, "search", "Alpha", "--vault", link, "--json", "--mode", "keyword")
	require.NoError(t, err)
	assert.Contains(t, search.String(), "concept-alpha")

	got, _, err := runRootCmd(t, "note", "get", "concept-alpha", "--vault", link, "--json")
	require.NoError(t, err)
	assert.Contains(t, got.String(), "concept-alpha")

	doc, _, err := runRootCmd(t, "doctor", "--vault", link, "--json")
	require.NoError(t, err)
	var doctor struct {
		Result struct {
			IndexStatus string `json:"index_status"`
			TotalFiles  int    `json:"total_files"`
			Issues      struct {
				OrphanedEntries int `json:"orphaned_entries"`
			} `json:"issues"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(doc.Bytes(), &doctor))
	assert.Equal(t, "current", doctor.Result.IndexStatus)
	assert.Equal(t, 0, doctor.Result.Issues.OrphanedEntries)
	assert.Positive(t, doctor.Result.TotalFiles)
}

// Emptying the vault and re-indexing must fail, leave the notes in the index,
// and say how to reset. Exit is the command error, which is non-zero.
func TestIndex_EmptiedVaultExitsNonZeroWithoutDeleting(t *testing.T) {
	isolateMeshEnv(t)
	vault := buildIndexedTestVault(t)
	require.NoError(t, filepath.WalkDir(vault, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".vaultmind" {
			return filepath.SkipDir
		}
		if !d.IsDir() && filepath.Ext(path) == ".md" {
			return os.Remove(path)
		}
		return nil
	}))

	_, _, err := runRootCmd(t, "index", "--vault", vault, "--json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing was deleted")
	assert.Contains(t, err.Error(), ".vaultmind/index.db")

	got, _, err := runRootCmd(t, "note", "get", "concept-alpha", "--vault", vault, "--json")
	require.NoError(t, err)
	assert.Contains(t, got.String(), "Alpha Concept")
}
