package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runInit on a fresh path scaffolds the vault and prints the
// next-steps message. The next-steps message is the user-facing
// onboarding bridge — if it goes silent, new users have no idea what
// to do next.
func TestRunInit_ScaffoldsAndPrintsNextSteps(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "fresh-vault")

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	require.NoError(t, runInit(cmd, []string{dst}))

	out := buf.String()
	assert.Contains(t, out, "Vault scaffolded at")
	assert.Contains(t, out, "vaultmind index")
	assert.Contains(t, out, "vaultmind ask")
	// .gitignore guidance: index is a regenerable cache, config is source
	// (issue #41 init ask — surface it so the SQLite index isn't committed).
	assert.Contains(t, out, ".vaultmind/index.db*")
	assert.Contains(t, out, "!.vaultmind/config.yaml")

	// Sanity: scaffold actually happened on disk.
	_, err := os.Stat(filepath.Join(dst, ".vaultmind", "config.yaml"))
	assert.NoError(t, err)
}

// runInit on an existing path surfaces the refusal as an error to the
// user — the underlying initvault.Init refuses, but cmd/init.go must
// propagate that cleanly rather than silently producing zero output.
func TestRunInit_RefusesExistingPath(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "occupied")
	require.NoError(t, os.MkdirAll(dst, 0o750))

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	err := runInit(cmd, []string{dst})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refuse to overwrite")
	assert.Empty(t, buf.String(), "no next-steps message should print on failure")
}

// TestInitPrintInstructions_DefaultsToQuickStart — `--print-instructions`
// without --full emits the concise quick-start (slice #4). Assert a
// quick-start-only marker is present and a full-guide-only marker is absent,
// so the default routes to the right doc.
func TestInitPrintInstructions_DefaultsToQuickStart(t *testing.T) {
	out, _, err := runRootCmd(t, "init", "--print-instructions")
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "# VaultMind — Quick Start",
		"--print-instructions must emit the concise quick-start by default")
	assert.NotContains(t, text, "## 5. Migration path",
		"the default quick-start must NOT contain full-guide-only sections")
}

// TestInitPrintInstructions_FullEmitsWholeGuide — `--print-instructions --full`
// emits the full agent-onboarding guide AND the generated grouped command
// reference (the catalog). Assert a full-guide-only marker, the command
// reference H1, and the absence of the quick-start H1.
func TestInitPrintInstructions_FullEmitsWholeGuide(t *testing.T) {
	out, _, err := runRootCmd(t, "init", "--print-instructions", "--full")
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "## 5. Migration path",
		"--print-instructions --full must emit the full guide")
	assert.Contains(t, text, "# VaultMind Commands",
		"--print-instructions --full must append the generated command reference")
	assert.NotContains(t, text, "# VaultMind — Quick Start",
		"the full guide must NOT be the quick-start")
}
