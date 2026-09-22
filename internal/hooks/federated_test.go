package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Federated recall existed in the scripts (VAULTMIND_VAULTS) but no install
// path could turn it on: the one federated project in use had hand-edited its
// settings. An installed feature nobody can install is not shipped.

func commandFor(t *testing.T, hs []canonicalHook, script string) string {
	t.Helper()
	for _, ch := range hs {
		if ch.Script == script {
			return ch.Group.Hooks[0].Command
		}
	}
	t.Fatalf("no hook for %s", script)
	return ""
}

func TestFederated_SearchHooksGetTheVaultList(t *testing.T) {
	hs := canonicalHooksFor("", []string{"/v/identity", "/v/desk"})
	for _, s := range []string{hookUserPromptSubmitScript, hookReachScript} {
		assert.Contains(t, commandFor(t, hs, s), "VAULTMIND_VAULTS='/v/identity,/v/desk' ", s)
	}
}

// The identity loader reads ONE vault — the self is not a federation.
func TestFederated_PersonaLoadsOnlyThePrimary(t *testing.T) {
	hs := canonicalHooksFor("", []string{"/v/identity", "/v/desk"})
	c := commandFor(t, hs, hookSessionStartScript)
	assert.NotContains(t, c, "VAULTMIND_VAULTS")
	assert.Contains(t, c, "VAULTMIND_VAULT='/v/identity' ", "first listed vault is the primary")
}

func TestFederated_ExplicitVaultWinsAsPrimary(t *testing.T) {
	hs := canonicalHooksFor("/v/desk", []string{"/v/identity", "/v/desk"})
	assert.Contains(t, commandFor(t, hs, hookSessionStartScript), "VAULTMIND_VAULT='/v/desk' ")
}

func TestFederated_NoListMeansTodaysOutput(t *testing.T) {
	a, err := SettingsStanza("/v/identity")
	require.NoError(t, err)
	b, err := renderStanza(canonicalHooksFor("/v/identity", nil))
	require.NoError(t, err)
	assert.Equal(t, a, b)
	assert.NotContains(t, a, "VAULTMIND_VAULTS")
}

func TestFederated_ReachesBothAgentsFiles(t *testing.T) {
	dir := t.TempDir()
	vs := []string{"/v/identity", "/v/desk"}
	cl, err := MergeIntoSettingsFor(dir, "", vs, false, false)
	require.NoError(t, err)
	cx, err := MergeIntoCodexHooksFor(dir, "", vs, ProfileFull, false)
	require.NoError(t, err)
	for _, path := range []string{cl.SettingsPath, cx.SettingsPath} {
		b, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.True(t, strings.Contains(string(b), "VAULTMIND_VAULTS='/v/identity,/v/desk'"), filepath.Base(path))
	}
}
