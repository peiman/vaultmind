package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// export is a data-out contract: with no log at all a consumer wants a valid
// empty stream, not an error. What it must NOT do is create the log as a side
// effect — `experiments.telemetry: off` promises nothing is written, and an
// export that writes a database to tell you there is nothing to export breaks
// that promise with a file on disk.
func TestRunExport_NoLogEmitsEmptyStreamWithoutCreatingOne(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	viper.Reset()

	cmd := exportCmd
	cmd.ResetFlags()
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("tier", "anonymous", "")
	require.NoError(t, cmd.Flags().Set("tier", "anonymous"))

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	require.NoError(t, runExport(cmd, nil))

	// The manifest is byte-identical in shape to a real export's.
	out := strings.TrimSpace(buf.String())
	require.NotEmpty(t, out)
	var manifest map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.SplitN(out, "\n", 2)[0]), &manifest))
	assert.Equal(t, "manifest", manifest["kind"])
	assert.Equal(t, float64(0), manifest["session_count"])
	assert.Equal(t, float64(0), manifest["event_count"])

	_, statErr := os.Stat(filepath.Join(dataHome, "vaultmind", "experiments.db"))
	assert.True(t, os.IsNotExist(statErr),
		"exporting an absent log must not create one — that is a write under a tier that promises none")
}
