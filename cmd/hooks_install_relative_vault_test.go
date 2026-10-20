package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settingsCommands returns every hook command baked into a Claude settings file.
func settingsCommands(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc struct {
		Hooks map[string][]struct {
			Command string `json:"command"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(body, &doc))
	var cmds []string
	for _, groups := range doc.Hooks {
		for _, g := range groups {
			if g.Command != "" {
				cmds = append(cmds, g.Command)
			}
			for _, h := range g.Hooks {
				cmds = append(cmds, h.Command)
			}
		}
	}
	require.NotEmpty(t, cmds)
	return cmds
}

func commandWith(t *testing.T, cmds []string, needle string) string {
	t.Helper()
	for _, c := range cmds {
		if strings.Contains(c, needle) {
			return c
		}
	}
	t.Fatalf("no command contains %q\n%s", needle, strings.Join(cmds, "\n"))
	return ""
}

// A vault inside the project is written relative to $CLAUDE_PROJECT_DIR, which
// Claude Code expands. Teams commit settings.json; an absolute path is one
// machine's checkout. A vault outside the project stays absolute (see
// TestHooksInstall_RelativeVaultIsWrittenAbsolute).
func TestHooksInstall_InProjectVaultIsProjectRelative(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	proj := t.TempDir()
	knowledge := filepath.Join(proj, "knowledge")
	require.NoError(t, os.MkdirAll(knowledge, 0o750))
	t.Chdir(proj)

	_, _, err := runRootCmd(t, "hooks", "install", ".", "--vault", "./knowledge", "--merge")
	require.NoError(t, err)

	settings := filepath.Join(proj, ".claude", "settings.json")
	body, err := os.ReadFile(settings)
	require.NoError(t, err)
	cmds := settingsCommands(t, settings)
	health := commandWith(t, cmds, "vaultmind-health.sh")
	assert.Contains(t, health, `VAULTMIND_VAULT="$CLAUDE_PROJECT_DIR/knowledge"`)
	assert.NotContains(t, string(body), knowledge, "the absolute checkout path is not committed")

	out, _, err := runRootCmd(t, "hooks", "status", proj)
	require.NoError(t, err, out.String())
	assert.Contains(t, out.String(), "0 drifted, 0 missing")
	assert.Contains(t, out.String(), "0 unwired")

	// An install from before this change baked the absolute path. Status must
	// still call that in sync: drift is the scripts, not the spelling of the vault.
	old := strings.ReplaceAll(string(body), `VAULTMIND_VAULT=\"$CLAUDE_PROJECT_DIR/knowledge\"`, `VAULTMIND_VAULT='`+knowledge+`'`)
	require.NotEqual(t, string(body), old, "the rewrite must find the relative assignment")
	require.NoError(t, os.WriteFile(settings, []byte(old), 0o600))
	out, _, err = runRootCmd(t, "hooks", "status", proj)
	require.NoError(t, err, out.String())
	assert.Contains(t, out.String(), "0 drifted, 0 missing")
	assert.Contains(t, out.String(), "0 unwired")

	// Restore the relative form and run the installed SessionStart command the
	// way Claude Code does: CLAUDE_PROJECT_DIR set, the command taken from the file.
	require.NoError(t, os.WriteFile(settings, body, 0o600))
	shell := exec.Command("/bin/bash", "-c", health)
	shell.Stdin = strings.NewReader("{}")
	shell.Env = []string{
		"CLAUDE_PROJECT_DIR=" + proj,
		"PATH=/usr/bin:/bin",
		"HOME=" + t.TempDir(),
	}
	got, err := shell.Output()
	require.NoError(t, err, string(got))
	assert.Contains(t, string(got), "VaultMind vault detected ("+knowledge+")",
		"the hook must expand $CLAUDE_PROJECT_DIR and find the vault")

	// A teammate's checkout has a different absolute root. Run the same
	// command after moving the installed project, from an unrelated cwd.
	moved := filepath.Join(t.TempDir(), "checkout with 'quotes")
	require.NoError(t, os.Rename(proj, moved))
	shell = exec.Command("/bin/bash", "-c", health)
	shell.Dir = t.TempDir()
	shell.Stdin = strings.NewReader("{}")
	shell.Env = []string{"CLAUDE_PROJECT_DIR=" + moved, "PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	got, err = shell.Output()
	require.NoError(t, err, string(got))
	assert.Contains(t, string(got), "VaultMind vault detected ("+filepath.Join(moved, "knowledge")+")")
}

func TestHooksInstall_OtherAgentsUseProjectRelativeVaults(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, tc := range []struct{ agent, file string }{
		{"codex", ".codex/hooks.json"}, {"cursor", ".cursor/hooks.json"},
	} {
		t.Run(tc.agent, func(t *testing.T) {
			project := t.TempDir()
			knowledge := filepath.Join(project, "knowledge")
			outside := t.TempDir()
			require.NoError(t, os.MkdirAll(knowledge, 0o750))
			_, _, err := runRootCmd(t, "hooks", "install", project, "--agent", tc.agent, "--merge", "--vaults", knowledge+","+outside)
			require.NoError(t, err)
			cmds := settingsCommands(t, filepath.Join(project, tc.file))
			health := commandWith(t, cmds, "vaultmind-health.sh")
			assert.Contains(t, health, `VAULTMIND_VAULT="$CLAUDE_PROJECT_DIR/knowledge"`)
			federated := commandWith(t, cmds, "VAULTMIND_VAULTS=")
			assert.Contains(t, federated, `VAULTMIND_VAULTS="$CLAUDE_PROJECT_DIR/knowledge,`+outside+`"`)
			out, _, err := runRootCmd(t, "hooks", "status", project)
			if tc.agent == "cursor" {
				require.NoError(t, err, out.String())
			}
			// The separate approval check can fail for an untrusted install.
			assert.Contains(t, out.String(), "0 drifted, 0 missing")
			shell := exec.Command("/bin/bash", "-c", health)
			shell.Dir = project
			shell.Stdin = strings.NewReader(`{"workspace_roots":["` + project + `"]}`)
			shell.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
			got, err := shell.CombinedOutput()
			require.NoError(t, err, string(got))
			assert.Contains(t, string(got), "VaultMind vault detected ("+knowledge+")")
		})
	}
}

func TestHooksInstall_UserScopeVaultsStayAbsolute(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, tc := range []struct{ agent, file string }{
		{"claude", ".claude/settings.json"}, {"codex", ".codex/hooks.json"}, {"cursor", ".cursor/hooks.json"},
	} {
		t.Run(tc.agent, func(t *testing.T) {
			home := userHooksHome(t)
			knowledge := filepath.Join(home, "knowledge")
			outside := t.TempDir()
			require.NoError(t, os.MkdirAll(knowledge, 0o750))
			_, _, err := runRootCmd(t, "hooks", "install", "--scope", "user", "--agent", tc.agent, "--merge", "--vaults", knowledge+","+outside)
			require.NoError(t, err)
			cmds := settingsCommands(t, filepath.Join(home, tc.file))
			for _, c := range cmds {
				assert.NotContains(t, c, "$CLAUDE_PROJECT_DIR")
			}
			c := commandWith(t, cmds, "VAULTMIND_VAULTS=")
			assert.Contains(t, c, "VAULTMIND_VAULTS="+shellWord(knowledge+","+outside))
			assert.Contains(t, c, "VAULTMIND_VAULT="+shellWord(knowledge))
		})
	}
}

// --vaults gets the same rule, one path at a time. The assignment is
// double-quoted so the project-relative entry expands and the outside entry
// stays a literal absolute path.
func TestHooksInstall_VaultsListRelativizesOnlyInsideTheProject(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	proj := t.TempDir()
	knowledge := filepath.Join(proj, "knowledge")
	outside := filepath.Join(t.TempDir(), "other")
	require.NoError(t, os.MkdirAll(knowledge, 0o750))
	require.NoError(t, os.MkdirAll(outside, 0o750))
	t.Chdir(proj)

	_, _, err := runRootCmd(t, "hooks", "install", ".", "--vaults", "./knowledge,"+outside, "--merge")
	require.NoError(t, err)

	cmds := settingsCommands(t, filepath.Join(proj, ".claude", "settings.json"))
	federated := commandWith(t, cmds, "VAULTMIND_VAULTS=")
	assert.Contains(t, federated, `VAULTMIND_VAULTS="$CLAUDE_PROJECT_DIR/knowledge,`+outside+`"`)
	assert.Contains(t, federated, `VAULTMIND_VAULT="$CLAUDE_PROJECT_DIR/knowledge"`)
	assert.NotContains(t, federated, `VAULTMIND_VAULT='`+knowledge+`'`)
}
