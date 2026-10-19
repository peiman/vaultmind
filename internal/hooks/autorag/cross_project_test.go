package autorag_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The hardcoded cross-project-write signature denied two same-project writes
// for an agent (workhorse, 2026-10-17): its own session scratchpad, which
// macOS reports under /private/tmp (the default allowlist had /tmp), and a
// second checkout of the same repository. The deny also carried no guidance:
// the vault had nothing relevant to say.

func writeInput(path string) string {
	return `{"tool_name":"Write","tool_input":{"file_path":"` + path + `"}}`
}

// gitHermetic runs git without the caller's GIT_* (a hook running these tests
// from a worktree would otherwise point git at the real repository).
func gitHermetic(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func repoWithOrigin(t *testing.T, url string) string {
	t.Helper()
	dir := t.TempDir()
	gitHermetic(t, dir, "init", "-q")
	gitHermetic(t, dir, "remote", "add", "origin", url)
	return dir
}

// stubVaultmind writes a fake vaultmind whose ask prints the given header
// line, and a vault directory for it, so the guidance branch is exercised
// without a model.
func stubVaultmind(t *testing.T, searchLine string) (bin, vault string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "vaultmind")
	script := "#!/bin/bash\nprintf '%s\\n' '" + searchLine + "' '   1.  principle-stay-in-your-lane  Stay in your lane'\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o700))
	vault = filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(filepath.Join(vault, ".vaultmind"), 0o755))
	return bin, vault
}

// projectEnv pins the allowlist to the project alone. An empty
// AUTORAG_ALLOWED_ROOTS means "use the default", which includes /tmp, and on
// Linux every t.TempDir() is under /tmp.
func projectEnv(project string, extra ...string) []string {
	return append([]string{
		"CLAUDE_PROJECT_DIR=" + project,
		"VAULTMIND_PROJECT_DIR=" + project,
		"AUTORAG_ALLOWED_ROOTS=" + project,
	}, extra...)
}

// On macOS /tmp is a symlink to /private/tmp, and tools report the real path.
func TestGuard_ScratchpadUnderPrivateTmpIsNotCrossProject(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("/private/tmp is macOS's real path for /tmp")
	}
	project := t.TempDir()
	// The default allowlist (AUTORAG_ALLOWED_ROOTS unset) is what lists /tmp.
	out := runGuard(t, writeInput("/private/tmp/claude-501/-Users-x/session/scratchpad/patch.py"),
		"CLAUDE_PROJECT_DIR="+project, "VAULTMIND_PROJECT_DIR="+project)
	assert.Empty(t, out, "a write under /private/tmp (macOS's /tmp) is not cross-project")
}

func TestGuard_SameRemoteCheckoutIsNotCrossProject(t *testing.T) {
	project := repoWithOrigin(t, "git@github.com:peiman/workhorse.git")
	clone := repoWithOrigin(t, "https://github.com/peiman/workhorse")
	target := filepath.Join(clone, ".claude", "hooks", "changelog-guard.test.sh")

	out := runGuard(t, writeInput(target), projectEnv(project)...)
	assert.Empty(t, out, "another checkout of the same repository (same origin) is the same project")

	// An insteadOf rule can rewrite https://github.com/ to https://@github.com/
	// (an empty user part); that is still the same repository.
	rewritten := repoWithOrigin(t, "https://@github.com/peiman/workhorse")
	out = runGuard(t, writeInput(filepath.Join(rewritten, "main.go")), projectEnv(project)...)
	assert.Empty(t, out, "an empty user part in the origin URL is still the same repository")

	withPort := repoWithOrigin(t, "ssh://git@github.com:22/peiman/workhorse.git")
	out = runGuard(t, writeInput(filepath.Join(withPort, "main.go")), projectEnv(project)...)
	assert.Empty(t, out, "an ssh:// URL with a port is still the same repository")
}

func TestGuard_DifferentRepositoryWithGuidanceIsDenied(t *testing.T) {
	project := repoWithOrigin(t, "git@github.com:peiman/workhorse.git")
	other := repoWithOrigin(t, "git@github.com:peiman/someone-else.git")
	bin, vault := stubVaultmind(t, `Search: "cross-project boundary" (1 hits)  [relevance: strong (z=+3.10)]`)

	out := runGuard(t, writeInput(filepath.Join(other, "main.go")),
		projectEnv(project, "VAULTMIND_BIN="+bin, "AUTORAG_VAULT="+vault)...)
	assert.Contains(t, out, `"permissionDecision": "deny"`, "a real cross-project write with guidance is still denied")
	assert.Contains(t, out, "principle-stay-in-your-lane")
}

func TestGuard_CrossProjectWithoutRelevantGuidanceWarns(t *testing.T) {
	project := repoWithOrigin(t, "git@github.com:peiman/workhorse.git")
	other := repoWithOrigin(t, "git@github.com:peiman/someone-else.git")
	bin, vault := stubVaultmind(t, `Search: "cross-project boundary" (5 hits)  [relevance: nothing relevant — top hit at/below the off-topic noise floor (z=-1.03); body suppressed]`)

	out := runGuard(t, writeInput(filepath.Join(other, "main.go")),
		projectEnv(project, "VAULTMIND_BIN="+bin, "AUTORAG_VAULT="+vault)...)
	assert.NotContains(t, out, "permissionDecision", "a block that can't say what to do instead becomes a warning")
	assert.Contains(t, out, "additionalContext")
	assert.Contains(t, out, "cross-project-write")
	assert.Contains(t, out, "no vault guidance")
}

func TestGuard_CrossProjectWithNoVaultWarns(t *testing.T) {
	project := repoWithOrigin(t, "git@github.com:peiman/workhorse.git")
	other := repoWithOrigin(t, "git@github.com:peiman/someone-else.git")

	out := runGuard(t, writeInput(filepath.Join(other, "main.go")),
		projectEnv(project, "AUTORAG_VAULT="+filepath.Join(t.TempDir(), "absent"))...)
	assert.NotContains(t, out, "permissionDecision")
	assert.Contains(t, out, "no vault guidance")
}
