package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHooksInstallHelp_ProfileIsNotACommandMetavar(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	out, _, err := runRootCmd(t, "hooks", "install", "--help")
	require.NoError(t, err)
	text := out.String()
	assert.NotContains(t, text, "--profile hooks status")
	assert.Contains(t, text, "hooks status")
	assert.Contains(t, text, "--profile")
}

func TestDoctorHelp_MentionsGapsAndDrafts(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	out, _, err := runRootCmd(t, "doctor", "--help")
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "open gaps")
	assert.Contains(t, text, "draft")
	assert.Contains(t, text, "median days")
}
