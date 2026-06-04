package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPersistTelemetryChoice_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	err := persistTelemetryChoice(experiment.TelemetryAnonymous, configPath)
	require.NoError(t, err)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(data, &cfg))

	experiments, ok := cfg["experiments"].(map[string]any)
	require.True(t, ok, "should have experiments key")
	assert.Equal(t, experiment.TelemetryAnonymous, experiments["telemetry"])
}

func TestPersistTelemetryChoice_UpdatesExistingFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	// Write existing config with other settings
	existing := map[string]any{
		"app": map[string]any{
			"log": map[string]any{"level": "info"},
		},
		"experiments": map[string]any{
			"activation": map[string]any{"enabled": true},
		},
	}
	data, err := yaml.Marshal(existing)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, data, 0o600))

	err = persistTelemetryChoice(experiment.TelemetryOff, configPath)
	require.NoError(t, err)

	updated, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(updated, &cfg))

	// Telemetry should be set
	experiments := cfg["experiments"].(map[string]any)
	assert.Equal(t, experiment.TelemetryOff, experiments["telemetry"])

	// Existing keys should be preserved
	assert.NotNil(t, experiments["activation"], "existing experiment keys should be preserved")
	app := cfg["app"].(map[string]any)
	assert.NotNil(t, app["log"], "existing app config should be preserved")
}

func TestPersistTelemetryChoice_SecurePermissions(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	err := persistTelemetryChoice(experiment.TelemetryFull, configPath)
	require.NoError(t, err)

	info, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "config should be owner-only")
}
