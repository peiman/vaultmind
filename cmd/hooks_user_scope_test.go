package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func userHooksHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home with 'quote")
	require.NoError(t, os.MkdirAll(home, 0o750))
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Chdir(t.TempDir())
	return home
}

func TestHooksUserScope_InstallAbsoluteAndAdditive(t *testing.T) {
	home := userHooksHome(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(settings), 0o750))
	require.NoError(t, os.WriteFile(settings, []byte(`{"permissions":{"allow":["Bash(ls:*)"]},"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"bash /x/their-existing-hook.sh"}]}]}}`), 0o600))
	vault := filepath.Join(home, "knowledge")
	out, _, err := runRootCmd(t, "hooks", "install", "--scope", "user", "--vault", vault, "--merge", "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			ScriptsDir string `json:"scripts_dir"`
			Merge      struct {
				Path string `json:"settings_path"`
			} `json:"merge"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, filepath.Join(home, ".vaultmind", "scripts"), env.Result.ScriptsDir)
	assert.Equal(t, settings, env.Result.Merge.Path)
	require.FileExists(t, filepath.Join(env.Result.ScriptsDir, "vault-recall.sh"))
	first, err := os.ReadFile(settings)
	require.NoError(t, err)
	assert.Contains(t, string(first), "their-existing-hook.sh")
	assert.Contains(t, string(first), "permissions")
	assert.Contains(t, string(first), ".vaultmind/scripts/vault-recall.sh")
	assert.Contains(t, string(first), "VAULTMIND_VAULT=")
	assert.NotContains(t, string(first), "$CLAUDE_PROJECT_DIR", "user commands must never locate scripts in the active project")
	var wired struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(first, &wired))
	require.NotEmpty(t, wired.Hooks["UserPromptSubmit"])
	assert.Contains(t, wired.Hooks["UserPromptSubmit"][1].Hooks[0].Command, "bash "+shellWord(filepath.Join(home, ".vaultmind", "scripts", "vault-recall.sh")))
	// Run the emitted shell command from two unrelated projects, with a tiny
	// local probe in place of the installed recall script. HOME has spaces and
	// an apostrophe, proving both path independence and shell quoting.
	command := wired.Hooks["UserPromptSubmit"][1].Hooks[0].Command
	probe := filepath.Join(home, ".vaultmind", "scripts", "vault-recall.sh")
	original, err := os.ReadFile(probe)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(probe, []byte("#!/bin/bash\nprintf '%s' \"$VAULTMIND_VAULT\"\n"), 0o700))
	for _, project := range []string{t.TempDir(), t.TempDir()} {
		shell := exec.Command("/bin/bash", "-c", command)
		shell.Dir = project
		shell.Env = append(os.Environ(), "CLAUDE_PROJECT_DIR="+project)
		output, err := shell.CombinedOutput()
		require.NoError(t, err, string(output))
		assert.Equal(t, vault, strings.TrimSpace(string(output)))
	}
	require.NoError(t, os.WriteFile(probe, original, 0o700))

	_, _, err = runRootCmd(t, "hooks", "install", "--scope", "user", "--vault", vault, "--merge")
	require.NoError(t, err)
	second, err := os.ReadFile(settings)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestHooksUserScope_StatusAndProjectHint(t *testing.T) {
	home := userHooksHome(t)
	_, _, err := runRootCmd(t, "hooks", "install", "--scope", "user", "--merge")
	require.NoError(t, err)
	out, _, err := runRootCmd(t, "hooks", "status", "--scope", "user")
	require.NoError(t, err, out.String())
	assert.Contains(t, out.String(), filepath.Join(home, ".vaultmind", "scripts"))
	assert.Contains(t, out.String(), filepath.Join(home, ".claude", "settings.json"))
	assert.Contains(t, out.String(), "0 drifted, 0 missing")
	assert.Contains(t, out.String(), "0 unwired")
	project := t.TempDir()
	out, _, err = runRootCmd(t, "hooks", "status", project)
	require.ErrorIs(t, err, cmdutil.ErrAlreadyWritten)
	assert.Contains(t, out.String(), "User-scope hooks are installed too")
	assert.Contains(t, out.String(), "vaultmind hooks status --scope user")
	require.NoError(t, os.WriteFile(filepath.Join(home, ".vaultmind", "scripts", "vault-recall.sh"), []byte("#!/bin/bash\necho changed\n"), 0o700))
	out, _, err = runRootCmd(t, "hooks", "status", "--scope", "user", "--json")
	require.ErrorIs(t, err, cmdutil.ErrAlreadyWritten)
	assert.Contains(t, out.String(), `"state":"drifted"`)
	assert.Contains(t, out.String(), "vault-recall.sh")
}

func TestHooksUserScope_LegacyCustomScriptsAndDryRun(t *testing.T) {
	home := userHooksHome(t)
	script := filepath.Join(home, ".vaultmind", "scripts", "vault-recall.sh")
	require.NoError(t, os.MkdirAll(filepath.Dir(script), 0o750))
	custom := []byte("#!/bin/bash\necho custom\n")
	require.NoError(t, os.WriteFile(script, custom, 0o700))
	_, _, err := runRootCmd(t, "hooks", "install", "--scope", "user", "--merge")
	require.ErrorContains(t, err, "--force")
	settings := filepath.Join(home, ".claude", "settings.json")
	require.NoFileExists(t, settings)
	out, _, err := runRootCmd(t, "hooks", "install", "--scope", "user", "--merge", "--force", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Dry run")
	assert.Contains(t, out.String(), settings)
	assert.Contains(t, out.String(), ".vaultmind/scripts/vault-recall.sh")
	assert.NotContains(t, out.String(), "$CLAUDE_PROJECT_DIR")
	after, err := os.ReadFile(script)
	require.NoError(t, err)
	assert.Equal(t, custom, after)
	require.NoFileExists(t, settings)
	_, _, err = runRootCmd(t, "hooks", "install", "--scope", "user", "--merge", "--force")
	require.NoError(t, err)
	after, err = os.ReadFile(script)
	require.NoError(t, err)
	assert.NotEqual(t, custom, after)
}

func TestHooksUserScope_RejectRelativeVaultAndLocal(t *testing.T) {
	home := userHooksHome(t)
	for _, flags := range [][]string{{"--vault", "relative/vault"}, {"--local"}} {
		_, _, err := runRootCmd(t, append([]string{"hooks", "install", "--scope", "user"}, flags...)...)
		require.Error(t, err)
		if flags[0] == "--vault" {
			assert.Contains(t, err.Error(), "absolute")
			assert.Contains(t, err.Error(), "--vault")
		} else {
			assert.Contains(t, err.Error(), "--local")
		}
	}
	require.NoDirExists(t, filepath.Join(home, ".vaultmind"))
}

func TestHooksUserScope_DefaultProjectUnchanged(t *testing.T) {
	home := userHooksHome(t)
	project := t.TempDir()
	_, _, err := runRootCmd(t, "hooks", "install", project, "--merge")
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(project, ".claude", "scripts", "vault-recall.sh"))
	settings, err := os.ReadFile(filepath.Join(project, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Contains(t, string(settings), "$CLAUDE_PROJECT_DIR")
	require.NoDirExists(t, filepath.Join(home, ".vaultmind"))
}

func TestHooksUserScope_InvalidScope(t *testing.T) {
	userHooksHome(t)
	for _, command := range []string{"install", "status"} {
		_, _, err := runRootCmd(t, "hooks", command, "--scope", "global")
		require.ErrorContains(t, err, "--scope")
		assert.Contains(t, err.Error(), "project")
		assert.Contains(t, err.Error(), "user")
	}
}

func TestHooksUserScope_OtherAgents(t *testing.T) {
	for _, agent := range []string{"codex", "cursor"} {
		t.Run(agent, func(t *testing.T) {
			home := userHooksHome(t)
			out, _, err := runRootCmd(t, "hooks", "install", "--scope", "user", "--agent", agent, "--merge", "--dry-run")
			require.NoError(t, err)
			assert.Contains(t, out.String(), filepath.Join(home, "."+agent, "hooks.json"))
			assert.Contains(t, out.String(), ".vaultmind/scripts/")
			assert.NotContains(t, out.String(), "export VAULTMIND_PROJECT_DIR='", "never pin the active project to HOME")
			require.NoDirExists(t, filepath.Join(home, ".vaultmind"))
			_, _, err = runRootCmd(t, "hooks", "install", "--scope", "user", "--agent", agent, "--merge")
			require.NoError(t, err)
			out, _, err = runRootCmd(t, "hooks", "status", "--scope", "user", "--json")
			if agent == "codex" {
				require.ErrorIs(t, err, cmdutil.ErrAlreadyWritten)
				assert.Contains(t, out.String(), string(hooks.CodexNotApproved))
			} else {
				require.NoError(t, err)
			}
			assert.Contains(t, out.String(), `"`+agent+`":`)
		})
	}
}

func TestHooksUserScope_StanzaAndGuidance(t *testing.T) {
	home := userHooksHome(t)
	out, _, err := runRootCmd(t, "hooks", "install", "--scope", "user")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "vaultmind hooks install --scope user --merge")
	assert.Contains(t, out.String(), filepath.Join(home, ".claude", "settings.json"))
	assert.NotContains(t, out.String(), "$CLAUDE_PROJECT_DIR")
}

func TestHooksUserScope_CodexCustomHomeAndIdempotency(t *testing.T) {
	home := userHooksHome(t)
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	settings := filepath.Join(codexHome, "hooks.json")
	require.NoError(t, os.WriteFile(settings, []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"bash /x/foreign.sh"}]}]}}`), 0o600))
	_, _, err := runRootCmd(t, "hooks", "install", "--scope", "user", "--agent", "codex", "--merge")
	require.NoError(t, err)
	first, err := os.ReadFile(settings)
	require.NoError(t, err)
	assert.Contains(t, string(first), "foreign.sh")
	require.NoFileExists(t, filepath.Join(home, ".codex", "hooks.json"))
	_, _, err = runRootCmd(t, "hooks", "install", "--scope", "user", "--agent", "codex", "--merge")
	require.NoError(t, err)
	second, err := os.ReadFile(settings)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	out, _, err := runRootCmd(t, "hooks", "status", "--scope", "user")
	require.ErrorIs(t, err, cmdutil.ErrAlreadyWritten)
	assert.Contains(t, out.String(), settings)
	assert.NotContains(t, out.String(), "safe to delete", "shared user scripts are live")
}

func TestHooksUserScope_ProfileIndependentOfHomeProject(t *testing.T) {
	home := userHooksHome(t)
	_, _, err := runRootCmd(t, "hooks", "install", home, "--profile", "knowledge", "--merge")
	require.NoError(t, err)
	_, _, err = runRootCmd(t, "hooks", "install", "--scope", "user", "--profile", "persona", "--merge")
	require.NoError(t, err)
	out, _, err := runRootCmd(t, "hooks", "status", "--scope", "user")
	require.NoError(t, err, out.String())
	assert.Contains(t, out.String(), "[profile: persona]")
	_, _, err = runRootCmd(t, "hooks", "install", "--scope", "user", "--merge")
	require.NoError(t, err)
	declared, err := os.ReadFile(filepath.Join(home, ".vaultmind", "vaultmind-profile"))
	require.NoError(t, err)
	assert.Equal(t, "persona\n", string(declared))
}
