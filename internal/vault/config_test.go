package vault_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_ValidConfig(t *testing.T) {
	cfg, err := vault.LoadConfig("../../vaultmind-vault")
	require.NoError(t, err)

	assert.Contains(t, cfg.Vault.Exclude, ".git")
	assert.Contains(t, cfg.Vault.Exclude, ".obsidian")
	assert.NotEmpty(t, cfg.Types)

	projectType, ok := cfg.Types["project"]
	require.True(t, ok, "project type must exist")
	assert.Contains(t, projectType.Required, "status")
	assert.Contains(t, projectType.Required, "title")
	assert.Contains(t, projectType.Statuses, "active")
	assert.Equal(t, "templates/project.md", projectType.Template)
}

func TestLoadConfig_MissingConfig(t *testing.T) {
	cfg, err := vault.LoadConfig(t.TempDir())
	require.NoError(t, err, "missing config should return defaults, not error")

	assert.Contains(t, cfg.Vault.Exclude, ".git")
	assert.Contains(t, cfg.Vault.Exclude, ".obsidian")
	assert.Empty(t, cfg.Types)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	vmDir := filepath.Join(dir, ".vaultmind")
	require.NoError(t, os.MkdirAll(vmDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(vmDir, "config.yaml"),
		[]byte("invalid: yaml: [[["),
		0o644,
	))

	_, err := vault.LoadConfig(dir)
	assert.Error(t, err)
}

func TestLoadConfig_DefaultExcludes(t *testing.T) {
	cfg, err := vault.LoadConfig(t.TempDir())
	require.NoError(t, err)

	defaults := []string{".git", ".obsidian", ".trash"}
	for _, d := range defaults {
		assert.Contains(t, cfg.Vault.Exclude, d)
	}
}
