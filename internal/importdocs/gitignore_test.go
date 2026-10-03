package importdocs_test

import (
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitRepo is a work tree whose .gitignore leaves out build/ and *.tmp.md,
// with build/keep.md force-added (tracked although a pattern matches it).
func gitRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "demo-repo")
	write(t, filepath.Join(repo, ".gitignore"), "build/\n*.tmp.md\n")
	write(t, filepath.Join(repo, "docs", "a.md"), "# A\n\nKept.\n")
	write(t, filepath.Join(repo, "docs", "c.tmp.md"), "# C\n\nScratch.\n")
	write(t, filepath.Join(repo, "build", "b.md"), "# B\n\nGenerated.\n")
	write(t, filepath.Join(repo, "build", "keep.md"), "# Keep\n\nForce-added.\n")
	r, err := gogit.PlainInit(repo, false)
	require.NoError(t, err)
	wt, err := r.Worktree()
	require.NoError(t, err)
	require.NoError(t, wt.AddWithOptions(&gogit.AddOptions{Path: "build/keep.md"}))
	idx, err := r.Storer.Index()
	require.NoError(t, err)
	_, err = idx.Entry("build/keep.md")
	require.NoError(t, err, "the fixture needs build/keep.md tracked")
	return repo
}

func importedNotes(res *importdocs.Result) []string {
	var out []string
	for _, e := range res.Entries {
		if e.Action == importdocs.Added {
			out = append(out, e.Note)
		}
	}
	return out
}

// A repository import takes what the project keeps, not what git ignores:
// generated output and scratch files stay out, a force-added file stays in,
// and the report says how much was left out.
func TestImport_ARepoRootLeavesOutWhatGitIgnores(t *testing.T) {
	repo, vault := gitRepo(t), t.TempDir()

	res, err := importdocs.Import(importdocs.Source{Dir: repo, Repo: "demo-repo"}, vault, importdocs.Options{})
	require.NoError(t, err)

	notes := importedNotes(res)
	assert.Len(t, notes, 2, "docs/a.md and the tracked build/keep.md: %v", notes)
	assert.Contains(t, notes, filepath.Join("imported", "demo-repo", "docs", "a.md"))
	assert.Contains(t, notes, filepath.Join("imported", "demo-repo", "build", "keep.md"))

	var ignored importdocs.Entry
	for _, e := range res.Entries {
		if e.Action == importdocs.Skipped && e.Reason != "" {
			ignored = e
		}
	}
	assert.Contains(t, ignored.Reason, "git ignores")
}

// Naming an ignored folder is asking for it.
func TestImport_AnIgnoredFolderNamedExplicitlyIsImported(t *testing.T) {
	repo, vault := gitRepo(t), t.TempDir()

	res, err := importdocs.Import(importdocs.Source{Dir: filepath.Join(repo, "build"), Repo: "demo-repo", Prefix: "build"}, vault, importdocs.Options{})
	require.NoError(t, err)
	assert.Len(t, importedNotes(res), 2, "build/b.md and build/keep.md")
}

// Git reads the global ignore file at its default place ($XDG_CONFIG_HOME/
// git/ignore) when core.excludesfile is not set; go-git does not, so the
// import reads it itself.
func TestImport_TheDefaultGlobalIgnoreFileApplies(t *testing.T) {
	home, cfg := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	write(t, filepath.Join(cfg, "git", "ignore"), "*.draft.md\n")
	repo := gitRepo(t)
	write(t, filepath.Join(repo, "docs", "x.draft.md"), "# Draft\n")

	res, err := importdocs.Import(importdocs.Source{Dir: repo, Repo: "demo-repo"}, t.TempDir(), importdocs.Options{})
	require.NoError(t, err)
	assert.NotContains(t, importedNotes(res), filepath.Join("imported", "demo-repo", "docs", "x.draft.md"))
}

// Outside a git work tree nothing is filtered, as before.
func TestImport_WithoutGitEverythingIsImported(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "plain")
	write(t, filepath.Join(repo, ".gitignore"), "build/\n")
	write(t, filepath.Join(repo, "docs", "a.md"), "# A\n")
	write(t, filepath.Join(repo, "build", "b.md"), "# B\n")

	res, err := importdocs.Import(importdocs.Source{Dir: repo, Repo: "plain"}, t.TempDir(), importdocs.Options{})
	require.NoError(t, err)
	assert.Len(t, importedNotes(res), 2)
}
