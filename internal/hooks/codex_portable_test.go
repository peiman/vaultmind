package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/testgit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #372: a committed .codex/hooks.json must not carry the operator's absolute
// project path. At the root of a git work tree the command asks git at run
// time instead.

func gitProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testgit.Run(t, dir, "init", "-q")
	return dir
}

func codexCommands(t *testing.T, projectDir, vault string) []string {
	t.Helper()
	var cmds []string
	for _, ch := range codexHooks(projectDir, vault, nil, ProfileFull) {
		cmds = append(cmds, ch.Group.Hooks[0].Command)
	}
	require.NotEmpty(t, cmds)
	return cmds
}

func TestCodexPortable_GitRootProjectHasNoAbsolutePath(t *testing.T) {
	dir := gitProject(t)
	out, err := CodexHooksStanza(dir, "", ProfileFull)
	require.NoError(t, err)
	assert.NotContains(t, out, dir)
	real, _ := filepath.EvalSymlinks(dir)
	assert.NotContains(t, out, real)
	f := renderCodex(t, dir)
	for _, groups := range f.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				assert.True(t, strings.HasPrefix(h.Command, `export VAULTMIND_PROJECT_DIR="$(git rev-parse --show-toplevel)"; bash "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/`), h.Command)
			}
		}
	}
}

func TestCodexPortable_NonGitProjectKeepsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	out, err := CodexHooksStanza(dir, "", ProfileFull)
	require.NoError(t, err)
	assert.Contains(t, out, dir)
	assert.NotContains(t, out, "git rev-parse")
}

func TestCodexPortable_SubdirectoryOfARepoKeepsAbsolutePath(t *testing.T) {
	repo := gitProject(t)
	sub := filepath.Join(repo, "pkg")
	require.NoError(t, os.MkdirAll(sub, 0o750))
	out, err := CodexHooksStanza(sub, "", ProfileFull)
	require.NoError(t, err)
	assert.Contains(t, out, sub)
	assert.NotContains(t, out, "git rev-parse")
}

func TestCodexPortable_ProjectVaultAddsTheClaudeAlias(t *testing.T) {
	dir := gitProject(t)
	vault := filepath.Join(dir, "notes")
	ref, err := PortableVaultRef(dir, vault, ScopeProject)
	require.NoError(t, err)
	cmds := codexCommands(t, dir, ref)
	require.NotEmpty(t, cmds)
	for _, c := range cmds {
		assert.Contains(t, c, `export VAULTMIND_PROJECT_DIR="$(git rev-parse --show-toplevel)"; export CLAUDE_PROJECT_DIR="$VAULTMIND_PROJECT_DIR"; VAULTMIND_VAULT="$CLAUDE_PROJECT_DIR/notes" bash "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/`)
		assert.NotContains(t, c, dir)
	}
}

func TestCodexPortable_OutsideVaultStaysAbsolute(t *testing.T) {
	dir := gitProject(t)
	vault := t.TempDir()
	cmds := codexCommands(t, dir, vault)
	require.NotEmpty(t, cmds)
	for _, c := range cmds {
		assert.Contains(t, c, vault)
		assert.NotContains(t, c, "CLAUDE_PROJECT_DIR")
	}
}

func TestCodexPortable_UserScopeUnchanged(t *testing.T) {
	dir := gitProject(t)
	hooks := codexHooksForScope(dir, "", nil, ProfileFull, ScopeUser)
	require.NotEmpty(t, hooks)
	for _, ch := range hooks {
		assert.True(t, strings.HasPrefix(ch.Group.Hooks[0].Command, `export VAULTMIND_PROJECT_DIR="$PWD"; `))
	}
}

// The command must work where Codex runs it: from a subdirectory.
func TestCodexPortable_CommandRunsFromASubdirectory(t *testing.T) {
	repo := gitProject(t)
	scripts := filepath.Join(repo, ".vaultmind", "scripts")
	require.NoError(t, os.MkdirAll(scripts, 0o750))
	stub := "echo ran\necho \"PROJECT=$VAULTMIND_PROJECT_DIR\"\necho \"PWD=$PWD\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(scripts, hookHealthScript), []byte(stub), 0o600))
	sub := filepath.Join(repo, "a", "b")
	require.NoError(t, os.MkdirAll(sub, 0o750))

	var command string
	for _, ch := range codexHooks(repo, "", nil, ProfileFull) {
		if ch.Script == hookHealthScript {
			command = ch.Group.Hooks[0].Command
		}
	}
	require.NotEmpty(t, command)
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = sub
	// The command runs git. Under the repo's own commit hook GIT_DIR is set and
	// would send that query to the wrong repository (#329).
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	realRepo, err := filepath.EvalSymlinks(repo)
	require.NoError(t, err)
	assert.Contains(t, string(out), "ran")
	assert.Contains(t, string(out), "PROJECT="+realRepo+"\n")
	realSub, _ := filepath.EvalSymlinks(sub)
	var pwd string
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "PWD="); ok {
			pwd, _ = filepath.EvalSymlinks(v)
		}
	}
	assert.Equal(t, realSub, pwd, "the script ran in the subdirectory Codex started it from")
}

func TestCodexPortable_RecognisedAsOurs(t *testing.T) {
	dir := gitProject(t)
	vaultRef, err := PortableVaultRef(dir, filepath.Join(dir, "notes"), ScopeProject)
	require.NoError(t, err)
	for _, c := range []struct {
		vault  string
		vaults []string
	}{{"", nil}, {vaultRef, nil}, {vaultRef, []string{vaultRef, "/elsewhere/vault"}}} {
		hooks := codexHooks(dir, c.vault, c.vaults, ProfileFull)
		require.NotEmpty(t, hooks)
		for _, ch := range hooks {
			cmd := ch.Group.Hooks[0].Command
			assert.True(t, commandReferencesScript(cmd, ch.Script), cmd)
			assert.Equal(t, []string{ch.Script}, scriptsInCommand(cmd, AgentCodex), cmd)
			assert.Equal(t, ch.Script, vaultmindScriptIn(cmd), cmd)
		}
	}
}

func TestCodexPortable_HandWrittenVariantsAreNotOurs(t *testing.T) {
	s := hookHealthScript
	good := `export VAULTMIND_PROJECT_DIR="$(git rev-parse --show-toplevel)"; bash "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/` + s
	require.True(t, commandReferencesScript(good, s))
	for name, cmd := range map[string]string{
		"different export": `export VAULTMIND_PROJECT_DIR="$(pwd)"; bash "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/` + s,
		"export with cd":   `export VAULTMIND_PROJECT_DIR="$(cd /x && git rev-parse --show-toplevel)"; bash "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/` + s,
		"no export":        `bash "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/` + s,
		"extra word":       good + ` extra`,
		"substitution":     `export VAULTMIND_PROJECT_DIR="$(git rev-parse --show-toplevel)"; bash "$(git rev-parse --show-toplevel)"/.vaultmind/scripts/` + s,
		"other dir":        `export VAULTMIND_PROJECT_DIR="$(git rev-parse --show-toplevel)"; bash "$VAULTMIND_PROJECT_DIR"/other/scripts/` + s,
		"wrong alias":      `export VAULTMIND_PROJECT_DIR="$(git rev-parse --show-toplevel)"; export CLAUDE_PROJECT_DIR=/evil; bash "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/` + s,
		"nested script":    `export VAULTMIND_PROJECT_DIR="$(git rev-parse --show-toplevel)"; bash "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/x/` + s,
		// Review of #372: each of these was taken for ours before the exact-shape gate.
		"trailing statement": good + `; echo mine`,
		"trailing comment":   good + ` # mine`,
		"prefix assignment":  `export VAULTMIND_PROJECT_DIR="$(git rev-parse --show-toplevel)"; VAULTMIND_PROJECT_DIR=/elsewhere bash "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/` + s,
		"alias assignment":   `export VAULTMIND_PROJECT_DIR="$(git rev-parse --show-toplevel)"; CLAUDE_PROJECT_DIR=/evil bash "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/` + s,
		"shell option":       `export VAULTMIND_PROJECT_DIR="$(git rev-parse --show-toplevel)"; bash -x "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/` + s,
		"other shell":        `export VAULTMIND_PROJECT_DIR="$(git rev-parse --show-toplevel)"; sh "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/` + s,
		"direct execution":   `export VAULTMIND_PROJECT_DIR="$(git rev-parse --show-toplevel)"; "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/` + s,
		"portable as shell":  `"$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/bash /p/.vaultmind/scripts/` + s,
	} {
		assert.False(t, commandReferencesScript(cmd, s), name)
		assert.Empty(t, scriptsInCommand(cmd, AgentCodex), name)
	}
}

func TestCodexPortable_MergeUpgradesAbsoluteInPlaceThenIsStable(t *testing.T) {
	dir := gitProject(t)
	path := CodexHooksPath(dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	// An install from before the fix: absolute, with a user hook in between.
	old := `{"hooks":{"SessionStart":[
 {"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"export VAULTMIND_PROJECT_DIR='` + dir + `'; bash '` + dir + `/.vaultmind/scripts/` + hookHealthScript + `'"}]},
 {"hooks":[{"type":"command","command":"echo mine"}]}]}}`
	require.NoError(t, os.WriteFile(path, []byte(old), 0o600))

	res, err := MergeIntoCodexHooks(dir, "", ProfileFull, false)
	require.NoError(t, err)
	assert.True(t, res.Changed)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), dir, "the absolute path must be gone")
	assert.Less(t, strings.Index(string(raw), hookHealthScript), strings.Index(string(raw), "echo mine"), "kept its position")
	assert.Equal(t, 1, strings.Count(string(raw), hookHealthScript), "upgraded, not duplicated")

	again, err := MergeIntoCodexHooks(dir, "", ProfileFull, false)
	require.NoError(t, err)
	assert.False(t, again.Changed)
}

func TestCodexPortable_UninstallRemovesIt(t *testing.T) {
	dir := gitProject(t)
	res, err := MergeIntoCodexHooks(dir, "", ProfileFull, false)
	require.NoError(t, err)
	raw, err := os.ReadFile(res.SettingsPath)
	require.NoError(t, err)
	out, removed, err := RemoveStanza(raw)
	require.NoError(t, err)
	assert.NotEmpty(t, removed)
	assert.NotContains(t, string(out), ".vaultmind/scripts")
}

func TestCodexPortable_StatusAndApprovalSeeIt(t *testing.T) {
	dir := gitProject(t)
	_, err := MergeIntoCodexHooks(dir, "", ProfileFull, false)
	require.NoError(t, err)
	report, err := codexApprovals(dir, codexHomeWith(t, ""))
	require.NoError(t, err)
	assert.NotEmpty(t, report.Hooks)
	assert.Equal(t, 0, report.Approved())
	st, err := Status(dir)
	require.NoError(t, err)
	require.NotNil(t, st.Codex)
	assert.NotEmpty(t, st.Codex.Hooks)
}

func TestCodexPortable_GeneratedUserCommandMigrates(t *testing.T) {
	home := t.TempDir()
	cmd := `export VAULTMIND_PROJECT_DIR="$(git rev-parse --show-toplevel)"; bash "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/` + hookHealthScript
	out, ok := generatedUserCommand(cmd, home, AgentCodex)
	require.True(t, ok)
	assert.Contains(t, out, filepath.Join(home, ".vaultmind", "scripts", hookHealthScript))
}

// The portable script word is ours only behind our own export, even when a
// caller asks generatedUserCommand directly.
func TestCodexPortable_GeneratedUserCommandNeedsThePortableExport(t *testing.T) {
	home := t.TempDir()
	ref := `bash "$VAULTMIND_PROJECT_DIR"/.vaultmind/scripts/` + hookHealthScript
	for name, cmd := range map[string]string{
		"no export":  ref,
		"pwd export": `export VAULTMIND_PROJECT_DIR="$PWD"; ` + ref,
		"abs export": `export VAULTMIND_PROJECT_DIR='/abs'; ` + ref,
		"trailing":   `export VAULTMIND_PROJECT_DIR="$(git rev-parse --show-toplevel)"; ` + ref + `; echo mine`,
	} {
		_, ok := generatedUserCommand(cmd, home, AgentCodex)
		assert.False(t, ok, name)
	}
}
