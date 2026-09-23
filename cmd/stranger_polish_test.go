package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// STRANGER TEST (2026-09-23): a `go install` build printed
// "vaultmind version v0.7.2-…, commit , built at  (backend: go-cpu)". Module
// builds carry no VCS stamp, so the fields were empty and the line looked
// broken — on the first command a new user runs. Unknown parts are omitted.
func TestVersionLine_OmitsUnknownParts(t *testing.T) {
	assert.Equal(t, "v0.8.0", versionLine("v0.8.0", "", ""))
	assert.Equal(t, "v0.8.0, commit abc123", versionLine("v0.8.0", "abc123", ""))
	assert.Equal(t, "v0.8.0, commit abc123, built at 2026-09-23", versionLine("v0.8.0", "abc123", "2026-09-23"))
	assert.Equal(t, "v0.8.0, built at 2026-09-23", versionLine("v0.8.0", "", "2026-09-23"))
}

// init told a new user how to wire Claude Code and nothing about Codex.
func TestInit_NextStepsMentionCodex(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "v")
	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"init", dir})
	t.Cleanup(func() { RootCmd.SetArgs([]string{}) })
	require.NoError(t, RootCmd.Execute())
	_, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "--agent codex --merge")
}
