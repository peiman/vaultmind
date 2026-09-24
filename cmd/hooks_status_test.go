package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func projectWithScripts(t *testing.T, install func(scriptsDir string)) string {
	t.Helper()
	dir := t.TempDir()
	scripts := filepath.Join(dir, ".claude", "scripts")
	require.NoError(t, os.MkdirAll(scripts, 0o750))
	install(scripts)
	return dir
}

// wireAllCanonicalEvents writes a settings.json that runs every canonical
// script on its canonical event.
//
// Fixtures used to install scripts and stop there, and asserted that a project
// matching canonical byte-for-byte was a clean check. That assumption is the one
// an adopter disproved: they held the scripts, wired three of the events, and
// the write half never ran — every content comparison passed. Scripts on disk
// are not hooks; wired scripts are. A fixture that skips the wiring is testing a
// state `hooks install` never produces.
func wireAllCanonicalEvents(t *testing.T, projectDir string) {
	t.Helper()
	byEvent := map[string][]map[string]any{}
	for _, e := range hooks.CanonicalEventScripts() {
		byEvent[e.Event] = append(byEvent[e.Event], map[string]any{
			"hooks": []map[string]any{{
				"type":    "command",
				"command": "$CLAUDE_PROJECT_DIR/.claude/scripts/" + e.Script,
			}},
		})
	}
	body, err := json.Marshal(map[string]any{"hooks": byEvent})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, ".claude", "settings.json"), body, 0o600))
}

func writeCanonical(t *testing.T, scriptsDir string, names ...string) {
	t.Helper()
	for _, n := range names {
		body, ok := hookscripts.Get(n)
		require.True(t, ok, "unknown canonical script %q", n)
		require.NoError(t, os.WriteFile(filepath.Join(scriptsDir, n), body, 0o700))
	}
}

// A status command that always exits 0 cannot gate anything, and this one
// exists so drift stops depending on somebody reading a line.
func TestHooksStatus_ExitsNonZeroWhenAnythingIsOff(t *testing.T) {
	all := hookscripts.Names()
	dir := projectWithScripts(t, func(scripts string) {
		writeCanonical(t, scripts, all[0]) // one installed, the rest missing
	})

	out, _, err := runRootCmd(t, "hooks", "status", dir)
	require.Error(t, err, "missing scripts must fail the check")
	assert.ErrorIs(t, err, cmdutil.ErrAlreadyWritten, "the report is the output; exit code is the signal")
	assert.Contains(t, out.String(), "missing")
}

func TestHooksStatus_ExitsZeroWhenFullyInSync(t *testing.T) {
	dir := projectWithScripts(t, func(scripts string) {
		writeCanonical(t, scripts, hookscripts.Names()...)
	})
	wireAllCanonicalEvents(t, dir)

	out, _, err := runRootCmd(t, "hooks", "status", dir)
	require.NoError(t, err, "canonical scripts AND canonical wiring is a clean check")
	assert.Contains(t, out.String(), "0 drifted, 0 missing")
}

// The two states have different fixes: drift may be a local change worth
// upstreaming, absence is just an install. Naming them separately is the point.
func TestHooksStatus_NamesDriftedAndMissingSeparately(t *testing.T) {
	all := hookscripts.Names()
	require.GreaterOrEqual(t, len(all), 2)
	dir := projectWithScripts(t, func(scripts string) {
		writeCanonical(t, scripts, all[0])
		body, _ := hookscripts.Get(all[1])
		require.NoError(t, os.WriteFile(filepath.Join(scripts, all[1]),
			append(body, []byte("\necho local\n")...), 0o700))
	})

	out, _, _ := runRootCmd(t, "hooks", "status", dir)
	text := out.String()
	assert.Contains(t, text, "drifted   "+all[1])
	assert.Contains(t, text, "send it upstream", "drift names the upstream path, not just --force")
	assert.Contains(t, text, "missing", "and absence is reported as its own state")
}

func TestHooksStatus_NotInstalledSaysSo(t *testing.T) {
	dir := t.TempDir()
	out, _, err := runRootCmd(t, "hooks", "status", dir)
	require.Error(t, err)
	assert.Contains(t, out.String(), "No hook scripts installed")
	assert.Contains(t, out.String(), "hooks install")
}

func TestHooksStatus_JSONCarriesEveryScriptState(t *testing.T) {
	dir := projectWithScripts(t, func(scripts string) {
		writeCanonical(t, scripts, hookscripts.Names()...)
	})
	wireAllCanonicalEvents(t, dir)

	out, _, err := runRootCmd(t, "hooks", "status", dir, "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			Installed bool `json:"installed"`
			Scripts   []struct {
				Name  string `json:"name"`
				State string `json:"state"`
			} `json:"scripts"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.True(t, env.Result.Installed)
	assert.Len(t, env.Result.Scripts, len(hookscripts.Names()),
		"every canonical script gets a state, not just the interesting ones")
}

// A stat failure on the scripts path must surface, not be reported as a clean
// project. `.claude` as a FILE makes `.claude/scripts` unstattable (ENOTDIR),
// which is the shape a real permissions or symlink problem takes.
func TestHooksStatus_UnreadableScriptsPathIsAnError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude"), []byte("not a dir"), 0o600))

	_, _, err := runRootCmd(t, "hooks", "status", dir)
	require.Error(t, err, "an unreadable hooks path is not an empty success")
}

func TestHooksStatus_UnreadableScriptsPathJSONIsAnErrorEnvelope(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude"), []byte("not a dir"), 0o600))

	out, _, err := runRootCmd(t, "hooks", "status", dir, "--json")
	require.Error(t, err)
	var env struct {
		Status string `json:"status"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors)
	assert.Equal(t, "status_failed", env.Errors[0].Code)
}

// Every write in the renderer is checked; a truncated report must not be
// mistaken for a short one. Drives each write-error return in turn.
func TestRenderHooksStatus_PropagatesWriteErrors(t *testing.T) {
	all := hookscripts.Names()
	report := hooks.StatusReport{
		ProjectDir: "/tmp/p",
		Installed:  true,
		Scripts: []hooks.ScriptStatus{
			{Name: all[0], State: hooks.ScriptDrifted},
			{Name: all[1], State: hooks.ScriptMissing},
		},
	}
	for n := range 5 {
		err := renderHooksStatus(&failAfterNWriter{ok: n}, report)
		require.Errorf(t, err, "write #%d must propagate its failure", n)
	}
}

// The not-installed branch writes too, and its failure must also surface.
func TestRenderHooksStatus_NotInstalledPropagatesWriteError(t *testing.T) {
	err := renderHooksStatus(&failAfterNWriter{ok: 0}, hooks.StatusReport{ProjectDir: "/tmp/p"})
	require.Error(t, err)
}

// Codex skips an unapproved hook SILENTLY — the agent starts with no memory
// and nothing says why. hooks status is where it says so, and fails, because a
// check that only prints cannot gate a setup script.
func codexProject(t *testing.T) (dir, hooksFile string) {
	t.Helper()
	dir = projectWithScripts(t, func(scripts string) {
		writeCanonical(t, scripts, hookscripts.Names()...)
	})
	wireAllCanonicalEvents(t, dir)
	codexScripts := filepath.Join(dir, ".vaultmind", "scripts")
	require.NoError(t, os.MkdirAll(codexScripts, 0o750))
	writeCanonical(t, codexScripts, "capture-episode.sh")
	hooksFile = filepath.Join(dir, ".codex", "hooks.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(hooksFile), 0o750))
	require.NoError(t, os.WriteFile(hooksFile, []byte(`{"hooks":{"SessionEnd":[{"hooks":[
		{"type":"command","command":"bash '`+codexScripts+`/capture-episode.sh'"}]}]}}`), 0o600))
	return dir, hooksFile
}

func TestHooksStatus_UnapprovedCodexHooksFailAndSayHowToFix(t *testing.T) {
	dir, _ := codexProject(t)
	t.Setenv("CODEX_HOME", t.TempDir())

	out, _, err := runRootCmd(t, "hooks", "status", dir)
	require.ErrorIs(t, err, cmdutil.ErrAlreadyWritten)
	s := out.String()
	assert.Contains(t, s, "Codex: 0 of 1 VaultMind hooks approved")
	assert.Contains(t, s, "not approved  SessionEnd -> capture-episode.sh")
	assert.Contains(t, s, "/hooks", "the fix, named")
}

func TestHooksStatus_ApprovedCodexHooksPass(t *testing.T) {
	dir, _ := codexProject(t)
	approveAllCodexHooks(t, dir)

	out, _, err := runRootCmd(t, "hooks", "status", dir)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Codex: 1 of 1 VaultMind hooks approved")
}

// A Codex-only project: no "No hook scripts installed", no Claude Code events —
// its scripts are checked in .vaultmind/scripts and it passes once approved.
func TestHooksStatus_CodexOnlyProjectPassesWhenApprovedAndInSync(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runRootCmd(t, "hooks", "install", dir, "--agent", "codex", "--merge")
	require.NoError(t, err)
	approveAllCodexHooks(t, dir)

	out, _, err := runRootCmd(t, "hooks", "status", dir)
	require.NoError(t, err, out.String())
	s := out.String()
	assert.NotContains(t, s, "No hook scripts installed")
	assert.NotContains(t, s, "unwired")
	assert.Contains(t, s, "Codex: 5 of 5 VaultMind hooks approved")
	assert.Contains(t, s, ".vaultmind/scripts: 5 in sync")
	_, statErr := os.Stat(filepath.Join(dir, ".claude"))
	assert.True(t, os.IsNotExist(statErr), "a Codex install creates no .claude folder")
}

// approveAllCodexHooks records, in a fresh CODEX_HOME, the approval Codex
// itself would write for every VaultMind hook in dir: its key and current hash.
func approveAllCodexHooks(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("CODEX_HOME", t.TempDir())
	report, err := hooks.Status(dir)
	require.NoError(t, err)
	require.NotNil(t, report.Codex)
	var cfg strings.Builder
	for _, h := range report.Codex.Hooks {
		fmt.Fprintf(&cfg, "[hooks.state.%q]\ntrusted_hash = %q\n\n", h.Key, h.Hash)
	}
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"), []byte(cfg.String()), 0o600))
	t.Setenv("CODEX_HOME", home)
}

// An upgrade that rewrites a hook's command leaves Codex's old approval in
// place with the old hash; Codex skips the hook. status must say so rather
// than count the stale record as approval.
func TestHooksStatus_ACodexHookChangedAfterApprovalFails(t *testing.T) {
	dir, hooksFile := codexProject(t)
	approveAllCodexHooks(t, dir)
	raw, err := os.ReadFile(hooksFile) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(hooksFile, []byte(strings.Replace(string(raw), "bash '", "bash  '", 1)), 0o600))

	out, _, err := runRootCmd(t, "hooks", "status", dir)
	require.ErrorIs(t, err, cmdutil.ErrAlreadyWritten)
	assert.Contains(t, out.String(), "changed since you approved it  SessionEnd -> capture-episode.sh")
}

// `hooks uninstall --agent codex` is the inverse of the Codex install: our
// entries leave .codex/hooks.json, and with --remove-scripts our scripts leave
// .vaultmind/scripts. It used to know only Claude Code.
func TestHooksUninstall_Codex(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runRootCmd(t, "hooks", "install", dir, "--agent", "codex", "--merge")
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "hooks", "uninstall", dir, "--agent", "codex", "--remove-scripts")
	require.NoError(t, err)
	s := out.String()
	assert.Contains(t, s, filepath.Join(dir, ".codex", "hooks.json"))
	assert.Contains(t, s, "Removed 5 VaultMind hook entries")
	assert.Contains(t, s, "from .vaultmind/scripts/")
	raw, err := os.ReadFile(filepath.Join(dir, ".codex", "hooks.json")) // #nosec G304 -- test path
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "vaultmind")
}

func TestHooksUninstall_CodexRejectsLocal(t *testing.T) {
	_, _, err := runRootCmd(t, "hooks", "uninstall", t.TempDir(), "--agent", "codex", "--local")
	require.ErrorContains(t, err, "--local")
}
