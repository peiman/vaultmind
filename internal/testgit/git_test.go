package testgit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand_StripsEveryGitVariableAndPreservesOtherEnvironment(t *testing.T) {
	for key, value := range map[string]string{
		"GIT_DIR": "/must/not/use", "GIT_WORK_TREE": "/must/not/use",
		"GIT_INDEX_FILE": "/must/not/use", "GIT_COMMON_DIR": "/must/not/use",
		"GIT_CONFIG_COUNT": "1", "GIT_CONFIG_KEY_0": "core.bare", "GIT_CONFIG_VALUE_0": "true",
		"GIT_CONFIG_NOSYSTEM": "0", "GIT_FUTURE_VARIABLE": "also stripped",
		"GIT_AUTHOR_NAME": "inherited identity", "TESTGIT_KEEP": "value=with=equals",
	} {
		t.Setenv(key, value)
	}
	dir := t.TempDir()
	cmd := Command(t, dir, "status", "--porcelain")
	assert.Equal(t, dir, cmd.Dir)
	assert.Equal(t, []string{"git", "status", "--porcelain"}, cmd.Args)
	wantEnv := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			wantEnv = append(wantEnv, kv)
		}
	}
	wantEnv = append(wantEnv, "GIT_CONFIG_NOSYSTEM=1")
	assert.ElementsMatch(t, wantEnv, cmd.Env)
}

func TestCommand_HookEnvironmentCannotDamageAnotherWorktree(t *testing.T) {
	root := t.TempDir()
	repo, worktree := filepath.Join(root, "R"), filepath.Join(root, "W")
	require.NoError(t, os.Mkdir(repo, 0o750))
	Run(t, repo, "init", "-q")
	// The commit needs an identity, provided per invocation without HOME config.
	Run(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid",
		"-c", "commit.gpgsign=false", "-c", "core.hooksPath="+filepath.Join(root, "no-hooks"),
		"commit", "--allow-empty", "-qm", "seed")
	Run(t, repo, "worktree", "add", "-b", "testgit", worktree)
	gitDir, err := Command(t, worktree, "rev-parse", "--absolute-git-dir").Output()
	require.NoError(t, err)
	configPath := filepath.Join(repo, ".git", "config")
	configBefore, err := os.ReadFile(configPath)
	require.NoError(t, err)
	indexPath := filepath.Join(strings.TrimSpace(string(gitDir)), "index")
	indexBefore, err := os.ReadFile(indexPath)
	require.NoError(t, err)

	t.Setenv("GIT_DIR", strings.TrimSpace(string(gitDir)))
	t.Setenv("GIT_INDEX_FILE", indexPath)
	t.Setenv("GIT_WORK_TREE", worktree)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.bare")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	target := t.TempDir()
	Run(t, target, "init", "-q")
	assert.DirExists(t, filepath.Join(target, ".git"))
	configAfter, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, string(configBefore), string(configAfter), "hook's common config must be untouched")
	indexAfter, err := os.ReadFile(indexPath)
	require.NoError(t, err)
	assert.Equal(t, indexBefore, indexAfter, "hook's worktree index must be untouched")
}
