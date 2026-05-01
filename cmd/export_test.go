package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runExport must surface a clear error when no telemetry tier has been
// chosen and --tier wasn't passed. We don't want to silently default to
// anonymous (or anything else) — the user has to make a choice.
func TestRunExport_RequiresTier(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	viper.Reset()
	viper.Set(config.KeyExperimentsTelemetry, "")

	cmd := exportCmd
	cmd.ResetFlags()
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("tier", "", "")

	err := runExport(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tier")
}

// runExport with --tier=anonymous against a fresh empty DB writes a
// manifest line plus zero session/event/outcome lines.
func TestRunExport_AnonymousEmptyDB(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	viper.Reset()

	cmd := exportCmd
	cmd.ResetFlags()
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("tier", "anonymous", "")
	require.NoError(t, cmd.Flags().Set("tier", "anonymous"))

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	require.NoError(t, runExport(cmd, nil))

	out := strings.TrimSpace(buf.String())
	require.NotEmpty(t, out, "manifest line should be written even when DB is empty")
	var manifest map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &manifest))
	assert.Equal(t, "manifest", manifest["kind"])
	assert.Equal(t, experiment.TelemetryAnonymous, manifest["tier"])
	assert.EqualValues(t, 0, manifest["session_count"])
	assert.EqualValues(t, 0, manifest["event_count"])
}

// --output writes to file; stdout stays clean.
func TestRunExport_WritesToOutputFile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	viper.Reset()

	outPath := filepath.Join(t.TempDir(), "export.jsonl")

	cmd := exportCmd
	cmd.ResetFlags()
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("tier", "", "")
	require.NoError(t, cmd.Flags().Set("tier", "anonymous"))
	require.NoError(t, cmd.Flags().Set("output", outPath))

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	require.NoError(t, runExport(cmd, nil))

	assert.Empty(t, buf.String(), "stdout should be empty when --output is given")
	data, err := os.ReadFile(outPath) // #nosec G304 -- t.TempDir() path
	require.NoError(t, err)
	assert.NotEmpty(t, data, "output file should contain at least the manifest")
	assert.Contains(t, string(data), `"kind":"manifest"`)
}

// runExport falls back to experiments.telemetry from config when --tier
// is empty.
func TestRunExport_UsesConfiguredTierWhenFlagEmpty(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	viper.Reset()
	viper.Set(config.KeyExperimentsTelemetry, "anonymous")

	cmd := exportCmd
	cmd.ResetFlags()
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("tier", "", "")

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	require.NoError(t, runExport(cmd, nil))
	assert.Contains(t, buf.String(), `"tier":"anonymous"`)
}

// Tier=off must refuse — the export logic owns that contract; this is a
// command-level assertion that the error surfaces to the user.
func TestRunExport_OffTierRefuses(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	viper.Reset()

	cmd := exportCmd
	cmd.ResetFlags()
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("tier", "", "")
	require.NoError(t, cmd.Flags().Set("tier", "off"))

	err := runExport(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "off")
}

// --output pointing at an unwritable path surfaces a create-file error.
func TestRunExport_OutputPathUnwritable(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	viper.Reset()

	// Path under a non-existent directory — os.Create fails.
	badPath := filepath.Join(t.TempDir(), "no-such-dir", "export.jsonl")

	cmd := exportCmd
	cmd.ResetFlags()
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("tier", "", "")
	require.NoError(t, cmd.Flags().Set("tier", "anonymous"))
	require.NoError(t, cmd.Flags().Set("output", badPath))

	err := runExport(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create output file")
}
