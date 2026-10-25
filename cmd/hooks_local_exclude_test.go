package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/testgit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const localExcludeStart = "# vaultmind hooks install --local (do not edit inside this block)\n"
const localExcludeEnd = "# end vaultmind\n"

func localExcludeRepo(t *testing.T) (string, string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	testgit.Run(t, dir, "init")
	path := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "--git-path", "info/exclude")))
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	return dir, path
}

func readLocalExclude(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func TestHooksLocalExclude_SettingsLocalIgnoredLeavesGitStatusClean(t *testing.T) {
	dir, path := localExcludeRepo(t)
	// Model the usual project policy for settings.local.json. The installer
	// owns only the two new exclusions, not this existing settings policy.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("/.claude/settings.local.json\n"), 0o600))
	testgit.Run(t, dir, "add", ".gitignore")
	testgit.Run(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.test", "-c", "core.hooksPath=/dev/null", "commit", "-m", "test fixture")

	out, _, err := runRootCmd(t, "hooks", "install", dir, "--merge", "--local")
	require.NoError(t, err)
	assert.Empty(t, string(testgit.Run(t, dir, "status", "--porcelain")))
	assert.Contains(t, readLocalExclude(t, path), localExcludeStart+"/.claude/scripts/\n/.claude/vaultmind-profile\n"+localExcludeEnd)
	assert.Contains(t, out.String(), "kept out of git")
	assert.Contains(t, out.String(), path)
	assert.Contains(t, out.String(), ".claude/vaultmind-profile")
}

func TestHooksLocalExclude_Layouts(t *testing.T) {
	for _, layout := range []string{"subdirectory", "worktree", "submodule"} {
		t.Run(layout, func(t *testing.T) {
			dir, _ := localExcludeRepo(t)
			testgit.Run(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.test", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "test fixture")
			project := filepath.Join(dir, "nested", "project")
			prefix := "/nested/project"
			switch layout {
			case "worktree":
				project = filepath.Join(t.TempDir(), "checkout")
				testgit.Run(t, dir, "worktree", "add", "-b", "local-test", project)
				prefix = ""
			case "submodule":
				source, _ := localExcludeRepo(t)
				testgit.Run(t, source, "-c", "user.name=Test", "-c", "user.email=test@example.test", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "test fixture")
				testgit.Run(t, dir, "-c", "protocol.file.allow=always", "submodule", "add", source, "module")
				project = filepath.Join(dir, "module")
				prefix = ""
			default:
				require.NoError(t, os.MkdirAll(project, 0o750))
			}
			path := strings.TrimSpace(string(testgit.Run(t, project, "rev-parse", "--git-path", "info/exclude")))
			if !filepath.IsAbs(path) {
				path = filepath.Join(project, path)
			}
			_, _, err := runRootCmd(t, "hooks", "install", project, "--merge", "--local")
			require.NoError(t, err)
			assert.Contains(t, readLocalExclude(t, path), localExcludeStart+prefix+"/.claude/scripts/\n"+prefix+"/.claude/vaultmind-profile\n"+localExcludeEnd)
			assert.Empty(t, string(testgit.Run(t, project, "ls-files", "--others", "--exclude-standard", "--", ".claude/scripts", ".claude/vaultmind-profile")))
			_, _, err = runRootCmd(t, "hooks", "uninstall", project, "--local")
			require.NoError(t, err)
			assert.NotContains(t, readLocalExclude(t, path), localExcludeStart)
		})
	}
}

func TestHooksLocalExclude_IdempotentPreservesUserLinesAndUninstallRemovesOnlyBlock(t *testing.T) {
	for _, original := range []string{"# user\r\n/.claude/scripts/\r\nkeep-me", "# user\nkeep-me\n", ""} {
		t.Run(original, func(t *testing.T) {
			dir, path := localExcludeRepo(t)
			require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
			_, _, err := runRootCmd(t, "hooks", "install", dir, "--merge", "--local")
			require.NoError(t, err)
			first := readLocalExclude(t, path)
			assert.Equal(t, 1, strings.Count(first, localExcludeStart))
			assert.Equal(t, 1, strings.Count(first, "/.claude/scripts/"))
			assert.Contains(t, first, original)
			out, _, err := runRootCmd(t, "hooks", "install", dir, "--merge", "--local", "--json")
			require.NoError(t, err)
			assert.JSONEq(t, `[]`, localExcludeJSON(t, out.Bytes(), "added"))
			assert.Equal(t, first, readLocalExclude(t, path))
			// Even user bytes appended after installation must survive removal.
			require.NoError(t, os.WriteFile(path, []byte("# before\n"+first+"\n# after\r\n"), 0o600))
			_, _, err = runRootCmd(t, "hooks", "uninstall", dir, "--local")
			require.NoError(t, err)
			assert.Equal(t, "# before\n"+original+"\n# after\r\n", readLocalExclude(t, path))
			_, _, err = runRootCmd(t, "hooks", "uninstall", dir, "--local")
			require.NoError(t, err)
			assert.Equal(t, "# before\n"+original+"\n# after\r\n", readLocalExclude(t, path))
		})
	}
}

func localExcludeJSON(t *testing.T, data []byte, field string) string {
	t.Helper()
	var env struct {
		Result struct {
			GitExclude map[string]json.RawMessage `json:"git_exclude"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(data, &env))
	require.Contains(t, env.Result.GitExclude, field)
	return string(env.Result.GitExclude[field])
}

func TestHooksLocalExclude_JSONAndDryRunWritesNothing(t *testing.T) {
	for _, jsonOut := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "json"}[jsonOut], func(t *testing.T) {
			dir, path := localExcludeRepo(t)
			before := readLocalExclude(t, path)
			args := []string{"hooks", "install", dir, "--merge", "--local", "--dry-run"}
			if jsonOut {
				args = append(args, "--json")
			}
			out, _, err := runRootCmd(t, args...)
			require.NoError(t, err)
			assert.Equal(t, before, readLocalExclude(t, path))
			assert.NoDirExists(t, filepath.Join(dir, ".claude"))
			if jsonOut {
				assert.JSONEq(t, `["/.claude/scripts/","/.claude/vaultmind-profile"]`, localExcludeJSON(t, out.Bytes(), "added"))
				assert.Equal(t, "true", localExcludeJSON(t, out.Bytes(), "dry_run"))
			} else {
				assert.Contains(t, out.String(), "Would add")
				assert.Contains(t, out.String(), "/.claude/scripts/")
				assert.Contains(t, out.String(), "/.claude/vaultmind-profile")
				assert.Contains(t, out.String(), path)
			}
			args = []string{"hooks", "install", dir, "--merge", "--local", "--json"}
			out, _, err = runRootCmd(t, args...)
			require.NoError(t, err)
			assert.JSONEq(t, `["/.claude/scripts/","/.claude/vaultmind-profile"]`, localExcludeJSON(t, out.Bytes(), "added"))
			assert.JSONEq(t, `"`+filepath.ToSlash(path)+`"`, localExcludeJSON(t, out.Bytes(), "path"))
		})
	}
}

func TestHooksLocalExclude_SkipsWithoutGit(t *testing.T) {
	for _, missingGit := range []bool{false, true} {
		t.Run(map[bool]string{false: "non-git directory", true: "git missing from PATH"}[missingGit], func(t *testing.T) {
			dir := t.TempDir()
			reason := "not a git work tree"
			if missingGit {
				t.Setenv("PATH", t.TempDir())
				reason = "git not found on PATH"
			}
			for _, jsonOut := range []bool{false, true} {
				args := []string{"hooks", "install", dir, "--merge", "--local"}
				if jsonOut {
					args = append(args, "--json")
				}
				out, _, err := runRootCmd(t, args...)
				require.NoError(t, err)
				if jsonOut {
					assert.Contains(t, localExcludeJSON(t, out.Bytes(), "skipped_reason"), reason)
				} else {
					assert.Contains(t, out.String(), "git exclusion skipped")
					assert.Contains(t, out.String(), ".claude/scripts/")
					assert.Contains(t, out.String(), ".claude/vaultmind-profile")
					assert.Contains(t, out.String(), "in the tree")
					assert.Contains(t, out.String(), reason)
				}
			}
		})
	}
}

func TestHooksLocalExclude_WithoutLocalUnchanged(t *testing.T) {
	dir, path := localExcludeRepo(t)
	before := readLocalExclude(t, path)
	out, _, err := runRootCmd(t, "hooks", "install", dir, "--merge", "--json")
	require.NoError(t, err)
	assert.NotContains(t, out.String(), "git_exclude")
	assert.Equal(t, before, readLocalExclude(t, path))
	_, _, err = runRootCmd(t, "hooks", "uninstall", dir)
	require.NoError(t, err)
	assert.Equal(t, before, readLocalExclude(t, path))
}

func TestHooksLocalExclude_ExistingEntriesNeedNoBlock(t *testing.T) {
	dir, path := localExcludeRepo(t)
	original := "# personal policy\n/.claude/scripts/\n/.claude/vaultmind-profile"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
	_, _, err := runRootCmd(t, "hooks", "install", dir, "--local")
	require.NoError(t, err)
	assert.Equal(t, original, readLocalExclude(t, path))
	_, _, err = runRootCmd(t, "hooks", "uninstall", dir, "--local")
	require.NoError(t, err)
	assert.Equal(t, original, readLocalExclude(t, path))
}

func TestHooksLocalExclude_SubprojectsUninstallIndependently(t *testing.T) {
	dir, path := localExcludeRepo(t)
	before := readLocalExclude(t, path)
	for _, project := range []string{"first", "second"} {
		projectDir := filepath.Join(dir, project)
		require.NoError(t, os.MkdirAll(projectDir, 0o750))
		_, _, err := runRootCmd(t, "hooks", "install", projectDir, "--merge", "--local")
		require.NoError(t, err)
	}
	_, _, err := runRootCmd(t, "hooks", "uninstall", filepath.Join(dir, "first"), "--local")
	require.NoError(t, err)
	after := readLocalExclude(t, path)
	assert.NotContains(t, after, "/first/.claude/")
	assert.Contains(t, after, "/second/.claude/scripts/")
	assert.Contains(t, after, before)
	_, _, err = runRootCmd(t, "hooks", "uninstall", filepath.Join(dir, "second"), "--local")
	require.NoError(t, err)
	assert.Equal(t, before, readLocalExclude(t, path))
}

func TestHooksLocalExclude_UninstallKeepsOtherWorktreeClean(t *testing.T) {
	for _, fixture := range []struct{ rel, keep string }{
		{".", "scripts"}, {"nested project", "vaultmind-profile"},
	} {
		t.Run(fixture.keep, func(t *testing.T) {
			rel := fixture.rel
			dir, path := localExcludeRepo(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("settings.local.json\n"), 0o600))
			testgit.Run(t, dir, "add", ".gitignore")
			testgit.Run(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.test", "-c", "core.hooksPath=/dev/null", "commit", "-m", "test fixture")
			wt := filepath.Join(t.TempDir(), "linked checkout")
			testgit.Run(t, dir, "worktree", "add", "-b", "local-test", wt)
			for _, root := range []string{dir, wt} {
				project := filepath.Join(root, rel)
				require.NoError(t, os.MkdirAll(project, 0o750))
				_, _, err := runRootCmd(t, "hooks", "install", project, "--merge", "--local")
				require.NoError(t, err)
			}
			other := "scripts"
			if fixture.keep == "scripts" {
				other = "vaultmind-profile"
			}
			require.NoError(t, os.RemoveAll(filepath.Join(dir, rel, ".claude", other)))
			before := readLocalExclude(t, path)
			out, _, err := runRootCmd(t, "hooks", "uninstall", filepath.Join(wt, rel), "--local", "--remove-scripts")
			require.NoError(t, err)
			assert.Equal(t, before, readLocalExclude(t, path))
			assert.Contains(t, out.String(), "another worktree")
			assert.Empty(t, string(testgit.Run(t, dir, "status", "--porcelain")))
			// The final checkout can remove the block after the other install
			// is gone (remove-scripts leaves its profile marker behind).
			require.NoError(t, os.RemoveAll(filepath.Join(wt, rel, ".claude")))
			_, _, err = runRootCmd(t, "hooks", "uninstall", filepath.Join(dir, rel), "--local")
			require.NoError(t, err)
			assert.NotContains(t, readLocalExclude(t, path), localExcludeStart)
		})
	}
}

func TestHooksLocalExclude_UninstallNonGitWording(t *testing.T) {
	out, _, err := runRootCmd(t, "hooks", "uninstall", t.TempDir(), "--local")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Git exclude cleanup skipped:")
	assert.NotContains(t, out.String(), "are in the tree")
	assert.NotContains(t, out.String(), "git exclusion skipped:")
}
