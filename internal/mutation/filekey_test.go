package mutation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SetFileKey sets one frontmatter key on a file named by path — for files the
// index deliberately leaves out (episodes), which the id-resolving Mutator
// cannot reach. It must go through the same conflict-checked atomic write, and
// touch nothing but the one key.

const fileKeyNote = "---\nid: episode-x\ntype: episode\ntags:\n  - episode\n---\n\n# Episode\n\nBody stays exactly as it was.\n"

func TestSetFileKey_SetsTheKeyAndLeavesTheBodyAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "episode-x.md")
	require.NoError(t, os.WriteFile(path, []byte(fileKeyNote), 0o600))

	require.NoError(t, SetFileKey(path, "reviewed", "2026-09-25"))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(got), "reviewed: \"2026-09-25\"\n")
	assert.Contains(t, string(got), "id: episode-x\n")
	assert.Contains(t, string(got), "\n# Episode\n\nBody stays exactly as it was.\n")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "permissions preserved")
}

func TestSetFileKey_ErrorsOnAFileWithoutFrontmatter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.md")
	require.NoError(t, os.WriteFile(path, []byte("# no frontmatter\n"), 0o600))
	assert.Error(t, SetFileKey(path, "reviewed", "2026-09-25"))
}

func TestSetFileKey_ErrorsOnAMissingFile(t *testing.T) {
	assert.Error(t, SetFileKey(filepath.Join(t.TempDir(), "absent.md"), "reviewed", "x"))
}
