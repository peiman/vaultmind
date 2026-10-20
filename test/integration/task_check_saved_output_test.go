package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CI runs the full task check as its own step and saves the output. The
// structure test reads that file instead of running the full check again
// (#313: the second run timed out the 0.10.15 release merge at 10m).
func TestRunTaskCheckForStructure_ReadsSavedOutput(t *testing.T) {
	saved := filepath.Join(t.TempDir(), "task-check.txt")
	require.NoError(t, os.WriteFile(saved, []byte("Development Environment\nTests\n"), 0o600))
	t.Setenv(taskCheckOutputFileEnv, saved)
	// With no PATH, running `task` would fail at once: a pass proves the file was read.
	t.Setenv("PATH", "")

	assert.Equal(t, "Development Environment\nTests\n", runTaskCheckForStructure(t))
}
