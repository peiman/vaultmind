package hooks

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/testgit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitExcludeRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	testgit.Run(t, dir, "init")
	return dir, filepath.Join(dir, ".git", "info", "exclude")
}

func TestLocalGitExclude_RoundTripAndDryRun(t *testing.T) {
	dir, path := gitExcludeRepo(t)
	original := "# user bytes\r\nkeep-this"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o640))
	require.NoError(t, os.Chmod(path, 0o640))
	preview := localGitExclude(dir, false, true)
	require.Empty(t, preview.SkippedReason)
	assert.Equal(t, []string{"/.claude/scripts/", "/.claude/vaultmind-profile"}, preview.Added)
	installed := localGitExclude(dir, false, false)
	require.Empty(t, installed.SkippedReason)
	assert.Equal(t, preview.Added, installed.Added)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	assert.Empty(t, localGitExclude(dir, false, false).Added)
	removed := localGitExclude(dir, true, false)
	require.Empty(t, removed.SkippedReason)
	assert.Equal(t, installed.Added, removed.Removed)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(after))
}

func TestLocalGitExclude_MissingFileAndAdvisoryIOFailures(t *testing.T) {
	for _, fixture := range []string{"missing file", "missing info directory", "exclude is directory", "info is file", "read only info directory"} {
		t.Run(fixture, func(t *testing.T) {
			if fixture == "read only info directory" && (runtime.GOOS == "windows" || os.Geteuid() == 0) {
				t.Skip("directory permissions do not block writes here")
			}
			dir, path := gitExcludeRepo(t)
			require.NoError(t, os.Remove(path))
			switch fixture {
			case "missing info directory":
				require.NoError(t, os.Remove(filepath.Dir(path)))
			case "exclude is directory":
				require.NoError(t, os.Mkdir(path, 0o750))
			case "info is file":
				require.NoError(t, os.Remove(filepath.Dir(path)))
				require.NoError(t, os.WriteFile(filepath.Dir(path), []byte("user"), 0o600))
			case "read only info directory":
				require.NoError(t, os.WriteFile(path, []byte("# user\n"), 0o600))
				require.NoError(t, os.Chmod(filepath.Dir(path), 0o500))
				t.Cleanup(func() { require.NoError(t, os.Chmod(filepath.Dir(path), 0o750)) })
			}
			res := localGitExclude(dir, false, true)
			if fixture == "exclude is directory" || fixture == "info is file" {
				assert.NotEmpty(t, res.SkippedReason)
				assert.Empty(t, res.Added)
				return
			}
			if fixture == "read only info directory" {
				res = localGitExclude(dir, false, false)
				assert.NotEmpty(t, res.SkippedReason)
				assert.Empty(t, res.Added)
				return
			}
			require.Empty(t, res.SkippedReason)
			assert.NoFileExists(t, path, "preview must not create info/exclude")
			assert.Empty(t, localGitExclude(dir, true, false).Removed)
			assert.NoFileExists(t, path, "absent block must remain a no-op")
			res = localGitExclude(dir, false, false)
			require.Empty(t, res.SkippedReason)
			assert.FileExists(t, path)
		})
	}
}

func TestLocalGitExclude_IgnoresInheritedGitEnvironment(t *testing.T) {
	dir, path := gitExcludeRepo(t)
	other, otherPath := gitExcludeRepo(t)
	before, err := os.ReadFile(otherPath)
	require.NoError(t, err)
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git", "index"))
	res := localGitExclude(dir, false, false)
	require.Empty(t, res.SkippedReason)
	assert.Equal(t, path, res.Path)
	after, err := os.ReadFile(otherPath)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestLocalGitExclude_LiteralProjectPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows disallows glob characters in directory names")
	}
	dir, _ := gitExcludeRepo(t)
	project := filepath.Join(dir, "space [a]*?")
	require.NoError(t, os.Mkdir(project, 0o750))
	res := localGitExclude(project, false, false)
	require.Empty(t, res.SkippedReason)
	file := filepath.Join(project, ".claude", "vaultmind-profile")
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o750))
	require.NoError(t, os.WriteFile(file, []byte("knowledge\n"), 0o600))
	assert.Empty(t, string(testgit.Run(t, dir, "ls-files", "--others", "--exclude-standard")))
}

func TestEditGitExclude_RepairsExistingBlockAndPreservesOtherProjects(t *testing.T) {
	entries := []string{"/.claude/scripts/", "/.claude/vaultmind-profile"}
	other := gitExcludeStart + "\n/other/.claude/scripts/\n" + gitExcludeEnd + "\n"
	ours := gitExcludeStart + "\n" + entries[0] + "\n" + gitExcludeEnd + "\n"
	updated, added := editGitExclude(ours+other+"# user", entries, false)
	assert.Equal(t, entries[1:], added)
	assert.Equal(t, 2, strings.Count(updated, gitExcludeStart))
	assert.Contains(t, updated, other+"# user")
	removed, names := editGitExclude(updated, entries, true)
	assert.Equal(t, entries, names)
	assert.Equal(t, other+"# user", removed)
	// An incomplete block is not a block uninstall owns.
	incomplete := gitExcludeStart + "\n" + entries[0]
	removed, names = editGitExclude(incomplete, entries, true)
	assert.Empty(t, names)
	assert.Equal(t, incomplete, removed)
}
