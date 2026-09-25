package navigate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/navigate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A note says which code it is about with `paths:` in its frontmatter — globs
// relative to the repository root, `**` for any depth, and `repo:` in front
// for code in another repository. When an agent opens a file, the notes that
// cover it are the knowledge it needs, found by the path it is already
// reading rather than by guessing a query.

func TestMatchPath(t *testing.T) {
	for _, c := range []struct {
		pattern, rel string
		want         bool
	}{
		{"cmd/tree.go", "cmd/tree.go", true},
		{"cmd/tree.go", "cmd/tree_test.go", false},
		{"cmd/*.go", "cmd/tree.go", true},
		{"cmd/*.go", "cmd/sub/tree.go", false},
		{"internal/navigate/**", "internal/navigate/navigate.go", true},
		{"internal/navigate/**", "internal/navigate/deep/x.go", true},
		{"internal/navigate/**", "internal/navigator/x.go", false},
		{"internal/navigate/", "internal/navigate/navigate.go", true},
		{"**/*.sh", "internal/hookscripts/vault-reach.sh", true},
		{"**/*.sh", "vault-reach.sh", true},
		{"internal/**/embed.go", "internal/hookscripts/embed.go", true},
		{"internal/**/embed.go", "internal/embed.go", true},
		{"./cmd/tree.go", "cmd/tree.go", true},
	} {
		assert.Equal(t, c.want, navigate.MatchPath(c.pattern, c.rel), "%s vs %s", c.pattern, c.rel)
	}
}

func coveringDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	notes := []struct{ id, paths string }{
		{"lesson-reach", `["internal/hookscripts/vault-reach.sh"]`},
		{"decision-hooks", `["internal/hookscripts/**", "cmd/hooks_*.go"]`},
		{"cross-repo", `["vaultmind-oss:internal/navigate/**"]`},
		{"other-repo", `["focalc:internal/navigate/**"]`},
		{"single-string", `"docs/spec.md"`},
		{"no-paths", ``},
	}
	for _, n := range notes {
		_, err := db.Exec(`INSERT INTO notes (id, path, title, type, body_text, hash, mtime) VALUES (?, ?, ?, 'concept', 'Body sentence. More.', 'h', 0)`,
			n.id, "concepts/"+n.id+".md", n.id)
		require.NoError(t, err)
		if n.paths != "" {
			_, err = db.Exec(`INSERT INTO frontmatter_kv (note_id, key, value_json) VALUES (?, 'paths', ?)`, n.id, n.paths)
			require.NoError(t, err)
		}
	}
	return db
}

func ids(notes []navigate.Note) []string {
	var out []string
	for _, n := range notes {
		out = append(out, n.ID)
	}
	return out
}

func TestCovering_FindsTheNotesAboutAFile(t *testing.T) {
	db := coveringDB(t)

	got, err := navigate.Covering(db, navigate.CodeFile{Rel: "internal/hookscripts/vault-reach.sh", Repo: "vaultmind-oss"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"lesson-reach", "decision-hooks"}, ids(got))
	for _, n := range got {
		assert.NotEmpty(t, n.Line, "each covering note carries its line for the map")
	}
}

func TestCovering_ARepoPrefixMatchesOnlyThatRepo(t *testing.T) {
	db := coveringDB(t)

	got, err := navigate.Covering(db, navigate.CodeFile{Rel: "internal/navigate/navigate.go", Repo: "vaultmind-oss"})
	require.NoError(t, err)
	assert.Equal(t, []string{"cross-repo"}, ids(got), "focalc:… must not match a file in vaultmind-oss")
}

func TestCovering_AcceptsASinglePathString(t *testing.T) {
	db := coveringDB(t)

	got, err := navigate.Covering(db, navigate.CodeFile{Rel: "docs/spec.md", Repo: "x"})
	require.NoError(t, err)
	assert.Equal(t, []string{"single-string"}, ids(got))
}

func TestCovering_NothingCoversAnUnrelatedFile(t *testing.T) {
	db := coveringDB(t)

	got, err := navigate.Covering(db, navigate.CodeFile{Rel: "README.md", Repo: "vaultmind-oss"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestResolveCodeFile_IsRelativeToTheGitRootAndNamedByIt(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "my-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "internal", "x"), 0o750))
	file := filepath.Join(repo, "internal", "x", "y.go")

	got := navigate.ResolveCodeFile(file)
	assert.Equal(t, navigate.CodeFile{Rel: "internal/x/y.go", Repo: "my-repo"}, got)
}

func TestResolveCodeFile_AWorktreeGitFileCountsAsARoot(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "wt")
	require.NoError(t, os.MkdirAll(repo, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: /elsewhere\n"), 0o600))

	got := navigate.ResolveCodeFile(filepath.Join(repo, "a.go"))
	assert.Equal(t, navigate.CodeFile{Rel: "a.go", Repo: "wt"}, got)
}

func TestResolveCodeFile_OutsideARepoHasNoRepo(t *testing.T) {
	file := filepath.Join(t.TempDir(), "loose.go")

	got := navigate.ResolveCodeFile(file)
	assert.Equal(t, "", got.Repo)
	assert.Equal(t, "loose.go", got.Rel, "the file name alone, so only bare patterns can match")
}

// The repository is named by its origin remote, not by the folder it happens
// to be cloned into: the same repo is "vaultmind-oss" on one machine and
// "vaultmind" on another, and a `repo:` prefix must mean the same on both.
func TestResolveCodeFile_NamesTheRepoByItsOriginRemote(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "local-folder-name")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o750))
	config := "[core]\n\tbare = false\n[remote \"upstream\"]\n\turl = git@github.com:someone/other.git\n" +
		"[remote \"origin\"]\n\turl = git@github.com:peiman/vaultmind.git\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git", "config"), []byte(config), 0o600))

	got := navigate.ResolveCodeFile(filepath.Join(repo, "cmd", "tree.go"))
	assert.Equal(t, navigate.CodeFile{Rel: "cmd/tree.go", Repo: "vaultmind"}, got)
}

func TestResolveCodeFile_AWorktreeUsesItsMainRepositorysRemote(t *testing.T) {
	base := t.TempDir()
	main := filepath.Join(base, "main-clone")
	require.NoError(t, os.MkdirAll(filepath.Join(main, ".git", "worktrees", "feature"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(main, ".git", "config"),
		[]byte("[remote \"origin\"]\n\turl = https://github.com/peiman/vaultmind\n"), 0o600))
	wt := filepath.Join(base, "feature-worktree")
	require.NoError(t, os.MkdirAll(wt, 0o750))
	gitdir := filepath.Join(main, ".git", "worktrees", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+gitdir+"\n"), 0o600))

	got := navigate.ResolveCodeFile(filepath.Join(wt, "a.go"))
	assert.Equal(t, navigate.CodeFile{Rel: "a.go", Repo: "vaultmind"}, got)
}

// A run of ** is one **; unfolded, each extra one multiplied the search and a
// pathological pattern took seconds per file.
func TestMatchPath_ARunOfAnyDepthStaysFast(t *testing.T) {
	pattern := strings.Repeat("**/", 20) + "nomatch.go"
	rel := strings.Repeat("d/", 20) + "file.go"
	start := time.Now()
	assert.False(t, navigate.MatchPath(pattern, rel))
	assert.Less(t, time.Since(start), 100*time.Millisecond)
	assert.True(t, navigate.MatchPath("**/**/file.go", "a/b/file.go"))
}

// `vaultmind frontmatter set` writes a JSON array as a quoted string (#159);
// such a paths value must still be read as the list it was meant to be.
func TestCovering_ReadsAListWrittenAsAString(t *testing.T) {
	db := coveringDB(t)
	_, err := db.Exec(`INSERT INTO notes (id, path, title, type, body_text, hash, mtime) VALUES ('quoted', 'concepts/quoted.md', 'quoted', 'concept', 'Body.', 'h', 0)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO frontmatter_kv (note_id, key, value_json) VALUES ('quoted', 'paths', ?)`, `"[\"lib/q.go\"]"`)
	require.NoError(t, err)

	got, err := navigate.Covering(db, navigate.CodeFile{Rel: "lib/q.go", Repo: "x"})
	require.NoError(t, err)
	assert.Equal(t, []string{"quoted"}, ids(got))
}

// A path through a symlink names the same file as its target.
func TestResolveCodeFile_FollowsSymlinks(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "real-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.go"), []byte("x"), 0o600))
	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(repo, link))

	got := navigate.ResolveCodeFile(filepath.Join(link, "a.go"))
	assert.Equal(t, navigate.CodeFile{Rel: "a.go", Repo: "real-repo"}, got)
}
