// cmd/version_test.go

package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVersionCommandRegistered guards the P2 papercut from the focalc field
// report: `vaultmind version` errored with "unknown command" even though
// --help advertised it. The subcommand must be wired to RootCmd.
func TestVersionCommandRegistered(t *testing.T) {
	var found bool
	for _, c := range RootCmd.Commands() {
		if c.Name() == "version" {
			found = true
			break
		}
	}
	assert.True(t, found, "`version` subcommand must be registered (focalc report P2)")
}

// TestVersionCommandOutput pins the output to the same shape as the global
// --version flag: "<name> version <Version>, commit <Commit>, built at <Date>".
func TestVersionCommandOutput(t *testing.T) {
	origV, origC, origD := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = origV, origC, origD })
	Version, Commit, Date = "v9.9.9", "abc1234", "2026-01-01_00:00:00"

	buf := &bytes.Buffer{}
	versionCmd.SetOut(buf)
	require.NoError(t, versionCmd.RunE(versionCmd, []string{}))

	out := buf.String()
	assert.Contains(t, out, "version v9.9.9")
	assert.Contains(t, out, "commit abc1234")
	assert.Contains(t, out, "built at 2026-01-01_00:00:00")
}
