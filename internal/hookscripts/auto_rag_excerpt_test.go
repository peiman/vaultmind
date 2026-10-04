package hookscripts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The guard injects canonical guidance when it sees a drift, such as an embed
// pass started by hand. It asks with --excerpt, as the recall and reach hooks
// do: the decision-bearing passage (a note's Principle) is what the guidance
// is, and ask now prints whole bodies, so without it the injection would grow
// to the whole budget.
func TestAutoRAGGuard_AsksForExcerpts(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "argv.log")
	h := newHookEnv(t, argvRecordingStub(logPath))
	require.NoError(t, os.MkdirAll(filepath.Join(h.projectDir, "vaultmind-identity", ".vaultmind"), 0o750))

	stdin, err := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]string{"command": "vaultmind index --embed --vault vaultmind-identity"},
	})
	require.NoError(t, err)
	runHookScript(t, "auto-rag-guard.sh", h.env(false), string(stdin))

	raw, err := os.ReadFile(logPath) //nolint:gosec // temp path owned by this test
	require.NoError(t, err, "the guard never asked the vault")
	args := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	require.Equal(t, "ask", args[0])
	assert.Contains(t, args, "--excerpt")
}
