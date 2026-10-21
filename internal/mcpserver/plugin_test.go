package mcpserver_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/mcpserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginProjectVaultDiscovery(t *testing.T) {
	for _, name := range []string{"knowledge", "vaultmind-vault", "vaultmind-identity", "."} {
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			dir := filepath.Join(project, name, ".vaultmind")
			require.NoError(t, os.MkdirAll(dir, 0755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("types: {}"), 0644))
			cfg, err := mcpserver.ProjectConfig(project, "test", "unused")
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(project, name), cfg.Vault)
			assert.False(t, cfg.NoVault)
		})
	}
}

func TestPluginMCPWorksAfterSetupWithoutRestart(t *testing.T) {
	project := t.TempDir()
	bin := "/bin/echo"
	cfg, err := mcpserver.ProjectConfig(project, "test", bin)
	require.NoError(t, err)
	assert.True(t, cfg.NoVault)
	cs := connect(t, cfg)
	result := call(t, cs, "tree", map[string]any{})
	assert.True(t, result.IsError)
	assert.Contains(t, text(result), "no vault in this project")
	dir := filepath.Join(project, "knowledge/.vaultmind")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("types: {}"), 0644))
	result = call(t, cs, "tree", map[string]any{})
	assert.False(t, result.IsError)
	assert.Contains(t, text(result), "--vault="+filepath.Join(project, "knowledge"))
}
