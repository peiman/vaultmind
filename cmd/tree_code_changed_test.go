package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitCommitAt(t *testing.T, repo, date, subject string, files ...string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		// Run under a git hook, the inherited GIT_INDEX_FILE and friends would
		// point these commands at the calling repository.
		for _, kv := range os.Environ() {
			if !strings.HasPrefix(kv, "GIT_") || strings.HasPrefix(kv, "GIT_AUTHOR_") || strings.HasPrefix(kv, "GIT_COMMITTER_") {
				cmd.Env = append(cmd.Env, kv)
			}
		}
		cmd.Env = append(cmd.Env, "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		run("init", "-q")
	}
	run(append([]string{"add"}, files...)...)
	run("commit", "-q", "-m", subject)
}

// `tree --for` says when the file was committed to after a note about it was:
// the moment of reading the code is when a stale note does its harm.
func TestTree_ForSaysWhenTheCodeChangedAfterTheNote(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "code-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "internal"), 0o750))
	file := filepath.Join(repo, "internal", "a.go")
	require.NoError(t, os.WriteFile(file, []byte("package internal\n"), 0o600))
	gitCommitAt(t, repo, "2026-09-01T10:00:00Z", "feat: a", "internal/a.go")

	vault := testvault.IndexedFixtureVault(t)
	note := "---\nid: concept-about-a\ntype: concept\ntitle: About a.go\npaths:\n  - internal/*.go\n---\n\nWhy a.go is built this way.\n"
	require.NoError(t, os.WriteFile(filepath.Join(vault, "concepts", "about-a.md"), []byte(note), 0o600))
	gitCommitAt(t, vault, "2026-09-10T10:00:00Z", "vault: about a", "concepts/about-a.md")
	_, _, err := runRootCmd(t, "index", "--vault", vault)
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "tree", "--vault", vault, "--for", file)
	require.NoError(t, err)
	assert.NotContains(t, out.String(), "code changed", "the note is newer than the code")

	require.NoError(t, os.WriteFile(file, []byte("package internal\n\nfunc A() {}\n"), 0o600))
	gitCommitAt(t, repo, "2026-09-20T10:00:00Z", "fix: a retries", "internal/a.go")

	out, _, err = runRootCmd(t, "tree", "--vault", vault, "--for", file)
	require.NoError(t, err)
	assert.Contains(t, out.String(), `code changed since this note: 1 commit, latest 2026-09-20 "fix: a retries" — check it still holds`)

	out, _, err = runRootCmd(t, "tree", "--vault", vault, "--for", file, "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			Vaults []struct {
				Root struct {
					Dirs []struct {
						Notes []struct {
							ID          string `json:"id"`
							CodeChanged *struct {
								Commits int    `json:"commits"`
								Summary string `json:"summary"`
							} `json:"code_changed"`
						} `json:"notes"`
					} `json:"dirs"`
				} `json:"root"`
			} `json:"vaults"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	require.Len(t, env.Result.Vaults, 1)
	require.NotEmpty(t, env.Result.Vaults[0].Root.Dirs)
	require.NotEmpty(t, env.Result.Vaults[0].Root.Dirs[0].Notes)
	n := env.Result.Vaults[0].Root.Dirs[0].Notes[0]
	require.NotNil(t, n.CodeChanged)
	assert.Equal(t, 1, n.CodeChanged.Commits)
	assert.Contains(t, n.CodeChanged.Summary, "check it still holds", "the hook prints this string as given")
}
