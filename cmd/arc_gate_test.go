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
