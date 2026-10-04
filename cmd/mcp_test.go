package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The server's vaults are absolute, so a tool's command finds them whatever
// directory the MCP client started the server in; several vaults make a
// read set whose first is where writes go.
func TestMCPServerConfig_VaultsAreAbsolute(t *testing.T) {
	vault := testvault.IndexedFixtureVault(t)
	rel, err := filepath.Rel(mustGetwd(t), vault)
	require.NoError(t, err)

	cfg, err := mcpServerConfig(rel, "")
	require.NoError(t, err)
	assert.Equal(t, vault, cfg.Vault)
	assert.Empty(t, cfg.Vaults, "one vault reads and writes the same place")
	assert.NotNil(t, cfg.Run)

	other := testvault.IndexedFixtureVault(t)
	cfg, err = mcpServerConfig(".", rel+","+other)
	require.NoError(t, err)
	assert.Equal(t, vault, cfg.Vault)
	assert.Equal(t, []string{vault, other}, cfg.Vaults)
}

// A path that is not a vault fails when the server starts, not on the
// first tool call, which an MCP client would show as a bare tool error.
func TestMCPServerConfig_RefusesAPathThatIsNotAVault(t *testing.T) {
	_, err := mcpServerConfig(t.TempDir(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a vault")
}

// The command is wired and documents how a client starts it.
func TestMCP_HelpNamesTheClientSetup(t *testing.T) {
	out, _, err := runRootCmd(t, "mcp", "--help")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "claude mcp add")
	assert.Contains(t, out.String(), "mcpServers")
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return wd
}
