package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Before a push the reach hook asks the vault about what is being pushed —
// the subjects of the commits not yet on any remote — not one fixed sentence.
// Measured 2026-09-28 on the research vault, eight real pushes labelled before
// ranking: the fixed sentence put a relevant note in the top 5 for none of
// them (MRR 0.06); the subjects did for all eight (MRR 0.79).

const fixedPushQuery = "publication is a one-way gate"

// gitRepo makes a repository whose first commit is on a (simulated) remote
// and whose later commits are not. The inherited git location variables are
// dropped: a hook exports them, and git would otherwise act on the caller's
// repository.
func gitRepo(t *testing.T, onRemote string, local ...string) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(cleanGitEnv(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	git("init", "-q")
	git("-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", onRemote)
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	for _, s := range local {
		git("-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", s)
	}
	return dir
}

func cleanGitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

func pushReach(t *testing.T, cmd, cwd string) string {
	t.Helper()
	h := newHookEnv(t, echoStub)
	b, _ := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": cmd}, "cwd": cwd})
	out, _ := runHookScript(t, "vault-reach.sh", h.env(false), string(b))
	return out
}

func TestReachPush_AsksAboutTheCommitsBeingPushed(t *testing.T) {
	repo := gitRepo(t, "feat: already pushed", "fix(cli): the second thing", "feat: the third thing")
	out := pushReach(t, "git push", repo)
	assert.Contains(t, out, "the second thing")
	assert.Contains(t, out, "the third thing")
	assert.NotContains(t, out, "already pushed", "a commit on the remote is not being pushed")
	assert.NotContains(t, out, "fix(cli):", "the conventional-commit prefix is dropped")
	assert.NotContains(t, out, fixedPushQuery)
}

func TestReachPush_AtMostTheThreeNewestSubjects(t *testing.T) {
	repo := gitRepo(t, "base", "one", "two", "three", "four", "five")
	out := pushReach(t, "git push -u origin feature", repo)
	assert.Contains(t, out, "five")
	assert.Contains(t, out, "three")
	assert.NotContains(t, out, "two", "only the three newest")
}

// The repository is where the push runs: after a cd, or at git -C.
func TestReachPush_FollowsCdAndGitC(t *testing.T) {
	repo := gitRepo(t, "base", "pushed from elsewhere")
	elsewhere := t.TempDir()
	assert.Contains(t, pushReach(t, "cd "+repo+" && git push", elsewhere), "pushed from elsewhere")
	assert.Contains(t, pushReach(t, "git -C "+repo+" push origin main", elsewhere), "pushed from elsewhere")
}

// Nothing new to push (a tag, or already pushed): the last commit stands in.
func TestReachPush_WithNothingNewAsksAboutTheLastCommit(t *testing.T) {
	repo := gitRepo(t, "release 0.9.18")
	assert.Contains(t, pushReach(t, "git push origin v0.9.18", repo), "release 0.9.18")
}

// Outside a repository there are no subjects: the fixed sentence remains.
func TestReachPush_OutsideARepositoryKeepsTheFixedQuery(t *testing.T) {
	assert.Contains(t, pushReach(t, "git push", t.TempDir()), fixedPushQuery)
}

// A merge keeps the fixed sentence: its subject is on GitHub, and the hook
// makes no network call.
func TestReachPush_AMergeKeepsTheFixedQuery(t *testing.T) {
	assert.Contains(t, pushReach(t, "gh pr merge 204 --merge", t.TempDir()), fixedPushQuery)
}

// Only a real push fires. The words inside an echo or a grep used to.
func TestReachPush_MentioningGitPushIsNotAPush(t *testing.T) {
	assert.Empty(t, pushReach(t, `echo "then git push"`, t.TempDir()))
	assert.Empty(t, pushReach(t, `grep -rn "git push" docs/`, t.TempDir()))
}

// The last commit on main is often a merge; "Merge pull request #204 from…"
// says nothing about the work. The fallback skips merges too (a real merge:
// two parents, as GitHub makes them).
func TestReachPush_TheFallbackSkipsAMergeCommit(t *testing.T) {
	repo := gitRepo(t, "base")
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Env = append(cleanGitEnv(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	git("checkout", "-q", "-b", "feature")
	git("commit", "-q", "--allow-empty", "-m", "feat: the real work")
	git("checkout", "-q", "-")
	git("merge", "-q", "--no-ff", "-m", "Merge pull request #204 from x/feature", "feature")
	git("update-ref", "refs/remotes/origin/main", "HEAD")

	out := pushReach(t, "git push", repo)
	assert.Contains(t, out, "the real work")
	assert.NotContains(t, out, "Merge pull request")
}

// A push behind a wrapper is still a push (review of the parser): sudo, env,
// nice and friends, and git's own --git-dir / --work-tree, which locate the
// repository as -C does.
func TestReachPush_SeesThroughWrappersAndGitDirFlags(t *testing.T) {
	repo := gitRepo(t, "base", "wrapped push")
	elsewhere := t.TempDir()
	for _, cmd := range []string{
		"sudo git -C " + repo + " push",
		"env GIT_SSH_COMMAND=ssh git -C " + repo + " push",
		"env -i PATH=/usr/bin git -C " + repo + " push",
		"nice -n 10 git -C " + repo + " push",
		"command git -C " + repo + " push",
		"git --git-dir=" + repo + "/.git push",
		"git --git-dir " + repo + "/.git --work-tree " + repo + " push",
		"git --work-tree=" + repo + " push",
	} {
		assert.Contains(t, pushReach(t, cmd, elsewhere), "wrapped push", cmd)
	}
}
