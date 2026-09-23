package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// STRANGER TEST (2026-09-23). `vaultmind hooks install ./myproject`, exactly as
// the README says, then the printed next step `vaultmind hooks install --merge`
// — which dropped ./myproject. Run from where the user stood, it wired
// work/.claude/settings.json; `hooks status ./myproject` then read
// "0 wired, 7 unwired". A suggested command must be the command the user ran,
// plus the one flag it is suggesting.

func TestRerunCommand_KeepsWhatTheUserTyped(t *testing.T) {
	got := hooksInstallRerun(hooksInstallParams{projectDir: "./my project", vault: "/v/id"}, hooksAgentClaude, nil)
	assert.Equal(t, "vaultmind hooks install './my project' --vault /v/id", got)

	got = hooksInstallRerun(hooksInstallParams{projectDir: "."}, hooksAgentCodex, []string{"/a", "/b"})
	assert.Equal(t, "vaultmind hooks install --agent codex --vaults /a,/b", got,
		"the current directory needs no argument; the agent and federation must survive")

	got = hooksInstallRerun(hooksInstallParams{projectDir: "p", profile: "knowledge"}, hooksAgentClaude, nil)
	assert.Equal(t, "vaultmind hooks install p --profile knowledge", got)
}

func TestHooksInstallHuman_NextStepCarriesTheProjectDir(t *testing.T) {
	var buf bytes.Buffer
	res := &hooks.InstallResult{ProjectDir: "./myproject", ScriptsDir: "myproject/.claude/scripts", SettingsStanza: "{}"}
	writeHooksInstallHuman(&buf, res, nil, installGuidance{rerun: "vaultmind hooks install ./myproject"})
	out := buf.String()
	assert.Contains(t, out, "vaultmind hooks install ./myproject --merge --dry-run")
	assert.Contains(t, out, "vaultmind hooks install ./myproject --merge ")
	assert.NotContains(t, out, "vaultmind hooks install --merge", "the bare form wires the wrong directory")
}

// No --vault, and the default the hooks will fall back to is not there: say so
// before the JSON, or the hooks load nothing and nothing says why.
func TestMissingVaultWarning(t *testing.T) {
	proj := t.TempDir()
	msg := missingVaultWarning(proj, "", nil)
	require.NotEmpty(t, msg)
	assert.Contains(t, msg, filepath.Join(proj, "vaultmind-identity"))
	assert.Contains(t, msg, "--vault")

	require.NoError(t, os.Mkdir(filepath.Join(proj, "vaultmind-identity"), 0o750))
	assert.Empty(t, missingVaultWarning(proj, "", nil), "the default exists: nothing to warn about")
	assert.Empty(t, missingVaultWarning(t.TempDir(), "/v/id", nil), "an explicit vault is the user's choice")
	assert.Empty(t, missingVaultWarning(t.TempDir(), "", []string{"/a", "/b"}))
}

func TestHooksInstallHuman_WarnsAboutAMissingVaultBeforeTheJSON(t *testing.T) {
	var buf bytes.Buffer
	res := &hooks.InstallResult{ProjectDir: "p", ScriptsDir: "p/.claude/scripts", SettingsStanza: "{\"hooks\":{}}"}
	writeHooksInstallHuman(&buf, res, nil, installGuidance{rerun: "vaultmind hooks install p", missingVault: "⚠ no vault"})
	out := buf.String()
	w, j := strings.Index(out, "⚠ no vault"), strings.Index(out, "{\"hooks\"")
	require.NotEqual(t, -1, w)
	assert.Less(t, w, j, "a warning below 60 lines of JSON has already scrolled away")
}
