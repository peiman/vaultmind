package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvision_InstallAndMerge(t *testing.T) {
	dir := t.TempDir()
	vault := filepath.Join(dir, "v")
	prov, err := Provision(InstallConfig{ProjectDir: dir, VaultPath: vault}, true, false, false)
	require.NoError(t, err)
	require.NotNil(t, prov.Install)
	assert.NotEmpty(t, prov.Install.Written)
	require.NotNil(t, prov.Merge)
	assert.True(t, prov.Merge.Changed)

	b, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Contains(t, string(b), "vault-recall.sh", "hooks wired")
	assert.Contains(t, string(b), vault, "vault path baked")
}

func TestProvision_InstallOnlyWhenMergeFalse(t *testing.T) {
	dir := t.TempDir()
	prov, err := Provision(InstallConfig{ProjectDir: dir}, false, false, false)
	require.NoError(t, err)
	assert.NotEmpty(t, prov.Install.Written)
	assert.Nil(t, prov.Merge, "merge=false → no merge step")
	_, statErr := os.Stat(filepath.Join(dir, ".claude", "settings.json"))
	assert.True(t, os.IsNotExist(statErr), "no settings written when merge=false")
}

func TestProvision_SkipsMergeOnScriptConflict(t *testing.T) {
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, ".claude", "scripts")
	require.NoError(t, os.MkdirAll(scriptsDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(scriptsDir, "vault-recall.sh"), []byte("# different\n"), 0o700))

	prov, err := Provision(InstallConfig{ProjectDir: dir}, true, false, false)
	require.Error(t, err, "a script conflict must surface")
	assert.Nil(t, prov.Merge, "merge must be skipped on conflict — never wire unresolved scripts")
	_, statErr := os.Stat(filepath.Join(dir, ".claude", "settings.json"))
	assert.True(t, os.IsNotExist(statErr), "settings must not be written on conflict")
}

func TestProvision_DryRunDoesNotWriteSettings(t *testing.T) {
	dir := t.TempDir()
	prov, err := Provision(InstallConfig{ProjectDir: dir}, true, false, true)
	require.NoError(t, err)
	require.NotNil(t, prov.Merge)
	assert.True(t, prov.Merge.DryRun)
	_, statErr := os.Stat(filepath.Join(dir, ".claude", "settings.json"))
	assert.True(t, os.IsNotExist(statErr), "dry-run must not write settings")
}
