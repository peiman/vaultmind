package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
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

	out, _, err := runRootCmd(t, "hooks", "status", dir)
	require.NoError(t, err, "a project matching canonical exactly is a clean check")
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
