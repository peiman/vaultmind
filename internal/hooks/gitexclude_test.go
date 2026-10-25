package hooks

import (
	"fmt"
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
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
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

func TestLocalGitExclude_SymlinkedSubproject(t *testing.T) {
	dir, path := gitExcludeRepo(t)
	project := filepath.Join(dir, "a", "b")
	require.NoError(t, os.MkdirAll(project, 0o750))
	outside := t.TempDir()
	link := filepath.Join(outside, "x", "y", "l2")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o750))
	require.NoError(t, os.Symlink(project, link))
	res, err := Provision(InstallConfig{ProjectDir: link}, false, true, false)
	require.NoError(t, err)
	require.Empty(t, res.Install.GitExclude.SkippedReason)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "/a/b/.claude/scripts/")
	assert.NoDirExists(t, filepath.Join(outside, "x", ".git"))
}

func TestLocalGitExclude_RefusesInfoOutsideCommonDir(t *testing.T) {
	dir, path := gitExcludeRepo(t)
	require.NoError(t, os.RemoveAll(filepath.Dir(path)))
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Dir(path)))
	res := localGitExclude(dir, false, false)
	assert.Contains(t, res.SkippedReason, "outside git common directory")
	assert.Empty(t, res.Added)
	assert.NoFileExists(t, filepath.Join(outside, "exclude"))
}

func TestLocalGitExclude_HonorsDiscoveryCeiling(t *testing.T) {
	dir, path := gitExcludeRepo(t)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	ceiling := filepath.Join(dir, "fenced")
	project := filepath.Join(ceiling, "project")
	require.NoError(t, os.MkdirAll(project, 0o750))
	t.Setenv("GIT_CEILING_DIRECTORIES", ceiling)
	res := localGitExclude(project, false, false)
	assert.Contains(t, res.SkippedReason, "not a git work tree")
	assert.Empty(t, res.Added)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestLocalGitExclude_PreservesExcludeSymlink(t *testing.T) {
	for _, outside := range []bool{false, true} {
		t.Run(fmt.Sprint(outside), func(t *testing.T) {
			dir, path := gitExcludeRepo(t)
			target := filepath.Join(filepath.Dir(path), "shared-exclude")
			if outside {
				target = filepath.Join(t.TempDir(), "shared-exclude")
			}
			require.NoError(t, os.Rename(path, target))
			require.NoError(t, os.Symlink(target, path))
			before, err := os.ReadFile(target)
			require.NoError(t, err)
			res := localGitExclude(dir, false, false)
			if outside {
				assert.Contains(t, res.SkippedReason, "outside git common directory")
				assert.Empty(t, res.Added)
			} else {
				require.Empty(t, res.SkippedReason)
				data, err := os.ReadFile(target)
				require.NoError(t, err)
				assert.Contains(t, string(data), gitExcludeStart)
				res = localGitExclude(dir, true, false)
				require.Empty(t, res.SkippedReason)
				assert.NotEmpty(t, res.Removed)
			}
			link, err := os.Readlink(path)
			require.NoError(t, err)
			assert.Equal(t, target, link)
			after, err := os.ReadFile(target)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}
