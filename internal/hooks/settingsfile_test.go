package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSettings(t *testing.T, projectDir, name, content string) string {
	t.Helper()
	dir := filepath.Join(projectDir, ".claude")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func TestMergeIntoSettings_CreatesFileWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	res, err := MergeIntoSettings(dir, "", false, false)
	require.NoError(t, err)
	assert.True(t, res.Changed)
	assert.False(t, res.DryRun)
	assert.Equal(t, filepath.Join(dir, ".claude", "settings.json"), res.SettingsPath)

	written, err := os.ReadFile(res.SettingsPath)
	require.NoError(t, err)
	assert.Contains(t, string(written), hookUserPromptSubmitScript)
}

func TestMergeIntoSettings_PreservesForeignHooks(t *testing.T) {
	dir := t.TempDir()
	p := writeSettings(t, dir, "settings.json", `{
  "hooks": { "UserPromptSubmit": [ { "hooks": [ { "type": "command", "command": "bash /x/their.sh" } ] } ] }
}`)
	res, err := MergeIntoSettings(dir, "", false, false)
	require.NoError(t, err)
	assert.True(t, res.Changed)

	written, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Contains(t, string(written), "their.sh", "foreign hook preserved on disk")
	assert.Contains(t, string(written), hookUserPromptSubmitScript, "our hook merged on disk")
}

func TestMergeIntoSettings_DryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	res, err := MergeIntoSettings(dir, "", false, true)
	require.NoError(t, err)
	assert.True(t, res.DryRun)
	assert.NotEmpty(t, res.Merged, "dry-run returns the would-be content for preview")

	_, statErr := os.Stat(filepath.Join(dir, ".claude", "settings.json"))
	assert.True(t, os.IsNotExist(statErr), "dry-run must not create the file")
}

func TestMergeIntoSettings_LocalTargetsLocalFile(t *testing.T) {
	dir := t.TempDir()
	res, err := MergeIntoSettings(dir, "", true, false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".claude", "settings.local.json"), res.SettingsPath)
	_, err = os.Stat(res.SettingsPath)
	require.NoError(t, err, "settings.local.json must be created")
}

func TestMergeIntoSettings_IdempotentSecondRunNoChange(t *testing.T) {
	dir := t.TempDir()
	_, err := MergeIntoSettings(dir, "", false, false)
	require.NoError(t, err)
	res2, err := MergeIntoSettings(dir, "", false, false)
	require.NoError(t, err)
	assert.False(t, res2.Changed, "second merge writes nothing")
}

func TestRemoveFromSettings_StripsOursKeepsTheirs(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, "settings.json", `{
  "hooks": { "UserPromptSubmit": [ { "hooks": [ { "type": "command", "command": "bash /x/their.sh" } ] } ] }
}`)
	_, err := MergeIntoSettings(dir, "", false, false)
	require.NoError(t, err)

	res, err := RemoveFromSettings(dir, false, false)
	require.NoError(t, err)
	assert.True(t, res.Changed)
	assert.NotEmpty(t, res.Removed)

	written, err := os.ReadFile(res.SettingsPath)
	require.NoError(t, err)
	assert.Contains(t, string(written), "their.sh", "foreign hook survives uninstall on disk")
	assert.NotContains(t, string(written), hookUserPromptSubmitScript)
}

func TestRemoveFromSettings_AbsentFileIsNoOp(t *testing.T) {
	dir := t.TempDir()
	res, err := RemoveFromSettings(dir, false, false)
	require.NoError(t, err)
	assert.False(t, res.Changed)
	assert.Empty(t, res.Removed)
}

func TestRemoveFromSettings_RemoveScriptsDeletesInstalled(t *testing.T) {
	dir := t.TempDir()
	// Install scripts so there's something to delete.
	_, err := Install(InstallConfig{ProjectDir: dir})
	require.NoError(t, err)
	_, err = MergeIntoSettings(dir, "", false, false)
	require.NoError(t, err)

	res, err := RemoveFromSettings(dir, false, true)
	require.NoError(t, err)
	assert.NotEmpty(t, res.ScriptsDeleted, "installed scripts deleted")

	// The scripts dir should no longer contain our canonical scripts.
	_, statErr := os.Stat(filepath.Join(dir, ".claude", "scripts", hookUserPromptSubmitScript))
	assert.True(t, os.IsNotExist(statErr), "deleted script must be gone")
}
