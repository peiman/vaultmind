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

func TestRunArcGate_WiresCobraIOAndFlags(t *testing.T) {
	vault := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(vault, "arc.md"), []byte("---\nid: arc-check\ntype: arc\nstatus: accepted\ntitle: Check First\n---\n## Principle\nCheck before acting.\n"), 0o600))
	triggers := filepath.Join(t.TempDir(), "arc-triggers.yaml")
	require.NoError(t, os.WriteFile(triggers, []byte("- arc: arc-check\n  on: pretooluse\n  match: check\n"), 0o600))
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	t.Setenv("VAULTMIND_ARC_TRIGGERS", triggers)

	cmd := &cobra.Command{}
	cmd.Flags().String("vault", ".", "")
	cmd.Flags().String("event", "", "")
	require.NoError(t, cmd.Flags().Set("vault", vault))
	require.NoError(t, cmd.Flags().Set("event", "pretooluse"))
	cmd.SetIn(bytes.NewBufferString(`{"session_id":"cmd-session","tool_name":"Write","tool_input":{"text":"check"}}`))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	require.NoError(t, runArcGate(cmd, nil))
	assert.Contains(t, out.String(), `"permissionDecision":"deny"`)
	assert.Empty(t, errOut.String())
}

func TestRunArcGate_ArmWritesMarkerAndPrintsNothing(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", stateHome)
	cmd := &cobra.Command{}
	cmd.Flags().String("vault", ".", "")
	cmd.Flags().String("event", "", "")
	cmd.Flags().Bool("arm", false, "")
	require.NoError(t, cmd.Flags().Set("arm", "true"))
	cmd.SetIn(bytes.NewBufferString(`{"session_id":"cmd-arm"}`))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	require.NoError(t, runArcGate(cmd, nil))
	assert.Empty(t, out.String())
	assert.Empty(t, errOut.String())
	_, err := os.Stat(filepath.Join(stateHome, "vaultmind", "arc-gate-armed", "cmd-arm"))
	require.NoError(t, err)
}

func TestRunArcGate_BadArmInputAllowsWithDiagnostic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cmd := &cobra.Command{}
	cmd.Flags().String("vault", ".", "")
	cmd.Flags().String("event", "", "")
	cmd.Flags().Bool("arm", false, "")
	require.NoError(t, cmd.Flags().Set("arm", "true"))
	cmd.SetIn(bytes.NewBufferString("{"))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	require.NoError(t, runArcGate(cmd, nil))
	assert.Empty(t, out.String())
	assert.Contains(t, errOut.String(), "arc gate: invalid hook JSON")
}
