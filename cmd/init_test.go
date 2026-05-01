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
