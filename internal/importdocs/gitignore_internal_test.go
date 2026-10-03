package importdocs

import (
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
}

// The default global file is read as git reads it: comments and blank lines
// skipped, CRLF line ends tolerated.
func TestDefaultGlobalPatterns_ReadsTheFileLikeGit(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	writeFile(t, filepath.Join(cfg, "git", "ignore"), "# a comment\r\n\r\n*.draft.md\r\nscratch/\n")

	m := gitignore.NewMatcher(defaultGlobalPatterns())
	assert.True(t, m.Match([]string{"docs", "a.draft.md"}, false))
	assert.True(t, m.Match([]string{"scratch"}, true))
	assert.False(t, m.Match([]string{"docs", "a.md"}, false))
	assert.False(t, m.Match([]string{"# a comment"}, false), "a comment is not a pattern")
}

// Without XDG_CONFIG_HOME it is ~/.config/git/ignore; with no file, nothing.
func TestDefaultGlobalPatterns_FallsBackToHomeAndToNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	assert.Empty(t, defaultGlobalPatterns(), "no file, no patterns")

	writeFile(t, filepath.Join(home, ".config", "git", "ignore"), "*.tmp.md\n")
	m := gitignore.NewMatcher(defaultGlobalPatterns())
	assert.True(t, m.Match([]string{"x.tmp.md"}, false))
}

// core.excludesfile, when set, is the global file — the default place is
// then not read, as in git.
func TestNewGitFilter_CoreExcludesfileWinsOverTheDefaultFile(t *testing.T) {
	home, cfg := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	writeFile(t, filepath.Join(home, "excludes"), "*.mine.md\n")
	writeFile(t, filepath.Join(home, ".gitconfig"), "[core]\n\texcludesfile = "+filepath.Join(home, "excludes")+"\n")
	writeFile(t, filepath.Join(cfg, "git", "ignore"), "*.other.md\n")

	repo := t.TempDir()
	_, err := gogit.PlainInit(repo, false)
	require.NoError(t, err)
	root, err := filepath.EvalSymlinks(repo)
	require.NoError(t, err)

	f := newGitFilter(root)
	require.NotNil(t, f)
	assert.True(t, f.ignored("a.mine.md", false))
	assert.False(t, f.ignored("a.other.md", false), "the default file is not read when core.excludesfile is set")
}
