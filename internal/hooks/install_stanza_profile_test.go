package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `hooks install --profile knowledge` without --merge printed the stanza for
// every canonical hook: a knowledge project was told to paste the persona
// loader and episode capture, scripts the same install had declined to write.
func TestInstall_PrintsOnlyTheProfilesHooks(t *testing.T) {
	res, err := Install(InstallConfig{ProjectDir: t.TempDir(), Profile: ProfileKnowledge})
	require.NoError(t, err)

	assert.Contains(t, res.SettingsStanza, hookUserPromptSubmitScript)
	assert.NotContains(t, res.SettingsStanza, hookSessionStartScript, "no persona loader in a knowledge stanza")
	assert.NotContains(t, res.SettingsStanza, hookSessionEndScript, "no episode capture in a knowledge stanza")
}

// Unset, the profile is full, as before.
func TestInstall_PrintsEveryHookForTheFullProfile(t *testing.T) {
	res, err := Install(InstallConfig{ProjectDir: t.TempDir()})
	require.NoError(t, err)
	assert.Contains(t, res.SettingsStanza, hookSessionStartScript)
	assert.Contains(t, res.SettingsStanza, hookSessionEndScript)
}
